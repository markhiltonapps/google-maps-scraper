package web

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// resultsRowLimit caps how many leads the preview and share pages render;
// the CSV downloads always contain every row.
const resultsRowLimit = 2000

// resultsView is the data for the results preview and the public share page.
type resultsView struct {
	Job   Job
	Table leadsTable
	// Ready is true once the job finished and its results file exists.
	Ready bool
}

// Title is the job name, or a fallback for unnamed jobs.
func (v *resultsView) Title() string {
	if v.Job.Name != "" {
		return v.Job.Name
	}

	return "Untitled search"
}

// Terms joins the job's search terms for display.
func (v *resultsView) Terms() string {
	return strings.Join(v.Job.Data.Keywords, " · ")
}

// SharePath is the public link for the job's results.
func (v *resultsView) SharePath() string {
	return "/share/" + v.Job.ID
}

func (s *Server) loadResults(ctx context.Context, id string) (resultsView, error) {
	job, err := s.svc.Get(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return resultsView{}, ErrNotFound
		}

		return resultsView{}, fmt.Errorf("get job: %w", err)
	}

	view := resultsView{Job: job}
	if job.Status != StatusOK {
		return view, nil
	}

	csvPath, err := s.svc.GetCSV(ctx, id)
	if err != nil {
		// A finished job without a results file has nothing to show yet.
		return view, nil //nolint:nilerr // missing results are rendered as an empty state
	}

	file, err := os.Open(csvPath) //nolint:gosec // GetCSV returns a validated path rooted in the configured data directory.
	if err != nil {
		return resultsView{}, fmt.Errorf("open results: %w", err)
	}
	defer file.Close()

	view.Table, err = buildLeads(file, resultsRowLimit)
	if err != nil {
		return resultsView{}, fmt.Errorf("read results: %w", err)
	}

	view.Ready = true

	return view, nil
}

// previewJob renders the results table in a modal on the main page.
func (s *Server) previewJob(w http.ResponseWriter, r *http.Request) {
	s.renderResults(w, r, "static/templates/preview.html")
}

// shareJob renders the public, read-only results page for a job.
func (s *Server) shareJob(w http.ResponseWriter, r *http.Request) {
	s.renderResults(w, r, "static/templates/share.html")
}

func (s *Server) renderResults(w http.ResponseWriter, r *http.Request, tmplKey string) {
	if r.Method != http.MethodGet {
		http.Error(w, methodNotAllowedMessage, http.StatusMethodNotAllowed)

		return
	}

	id, ok := getIDFromRequest(r)
	if !ok {
		http.Error(w, "This link is not valid.", http.StatusNotFound)

		return
	}

	view, err := s.loadResults(r.Context(), id.String())
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "These results do not exist or were deleted.", http.StatusNotFound)

		return
	}

	if err != nil {
		log.Printf("results %s: %v", id, err) //nolint:gosec // id is a parsed UUID and cannot contain log control characters.
		http.Error(w, "internal server error", http.StatusInternalServerError)

		return
	}

	tmpl, ok := s.tmpl[tmplKey]
	if !ok {
		http.Error(w, "missing tpl", http.StatusInternalServerError)

		return
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, &view); err != nil {
		log.Printf("results %s: render: %v", id, err) //nolint:gosec // id is a parsed UUID and cannot contain log control characters.
		http.Error(w, "internal server error", http.StatusInternalServerError)

		return
	}

	_, _ = buf.WriteTo(w)
}

// resultsTemplateFuncs are used by the leads table partial.
var resultsTemplateFuncs = template.FuncMap{
	"displayURL": displayURL,
	"aiChatClass": func(v string) string {
		switch {
		case strings.HasPrefix(v, leadsYes):
			return "pill-yes"
		case strings.HasPrefix(v, "Possibly"):
			return "pill-maybe"
		case v == leadsNo:
			return "pill-no"
		default:
			return "pill-neutral"
		}
	},
}

// displayURL shortens a website for display: "https://www.acme.com/" -> "acme.com".
func displayURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}

	shown := strings.TrimPrefix(u.Host, "www.")
	if p := strings.TrimSuffix(u.Path, "/"); p != "" {
		shown += p
	}

	return shown
}
