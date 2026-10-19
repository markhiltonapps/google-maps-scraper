package web

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lookupJobRepo returns jobs by ID, like the real store.
type lookupJobRepo struct {
	mockJobRepo
}

func (r *lookupJobRepo) Get(_ context.Context, id string) (Job, error) {
	for i := range r.jobs {
		if r.jobs[i].ID == id {
			return r.jobs[i], nil
		}
	}

	return Job{}, sql.ErrNoRows
}

const (
	doneJobID    = "11111111-1111-1111-1111-111111111111"
	runningJobID = "22222222-2222-2222-2222-222222222222"
)

func newResultsTestServer(t *testing.T) *Server {
	t.Helper()

	dir := t.TempDir()

	csv := "title,category,phone,website,emails,review_rating,review_count,address,link,ai_chat,chat_widgets,ai_phone_signals\n" +
		"Dental Smiles,Dentist,(512) 555-0100,https://www.mydentalsmiles.com/,,4.900000,366,1 Main St,https://maps.example/1,yes,RevenueWell AI Chat,\n" +
		"<script>alert(1)</script>,Dentist,,javascript:alert(1),,4,1,2 Main St,,no,,\n"
	if err := os.WriteFile(filepath.Join(dir, doneJobID+".csv"), []byte(csv), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	repo := &lookupJobRepo{mockJobRepo{jobs: []Job{
		{ID: doneJobID, Name: "Austin dentists", Status: StatusOK, Data: JobData{Keywords: []string{"dentists in Austin TX"}}},
		{ID: runningJobID, Name: "Still going", Status: StatusWorking},
	}}}

	srv, err := New(NewService(repo, dir), ":0")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return srv
}

func serveResults(t *testing.T, srv *Server, target string, handler func(*Server, http.ResponseWriter, *http.Request)) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, target, http.NoBody)
	if strings.HasPrefix(target, "/share/") {
		req.SetPathValue("id", strings.TrimPrefix(target, "/share/"))
	}

	rec := httptest.NewRecorder()
	handler(srv, rec, requestWithID(req))

	return rec
}

func TestShareJobRendersLeads(t *testing.T) {
	srv := newResultsTestServer(t)

	rec := serveResults(t, srv, "/share/"+doneJobID, (*Server).shareJob)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	for _, want := range []string{
		"Austin dentists",
		"dentists in Austin TX",
		"Dental Smiles",
		">mydentalsmiles.com</a>",
		`class="pill pill-yes">Yes</span>`,
		"/download?id=" + doneJobID + "&format=leads",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("share page missing %q", want)
		}
	}

	// The public page must not expose the app's controls.
	for _, unwanted := range []string{`hx-post="/scrape"`, "hx-delete"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("share page must not contain %q", unwanted)
		}
	}

	// Scraped data is untrusted: it must be escaped and unsafe links neutralized.
	if strings.Contains(body, "<script>alert(1)</script>") || strings.Contains(body, `href="javascript:`) {
		t.Fatal("share page rendered unescaped scraped content")
	}
}

func TestShareJobStillRunning(t *testing.T) {
	srv := newResultsTestServer(t)

	rec := serveResults(t, srv, "/share/"+runningJobID, (*Server).shareJob)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "still running") {
		t.Fatal("expected the still-running state")
	}
}

func TestShareJobNotFound(t *testing.T) {
	srv := newResultsTestServer(t)

	for _, target := range []string{"/share/33333333-3333-3333-3333-333333333333", "/share/not-a-uuid"} {
		if rec := serveResults(t, srv, target, (*Server).shareJob); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: got %d, want 404", target, rec.Code)
		}
	}
}

func TestPreviewJobRendersModal(t *testing.T) {
	srv := newResultsTestServer(t)

	rec := serveResults(t, srv, "/preview?id="+doneJobID, (*Server).previewJob)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	for _, want := range []string{`id="results-modal"`, `data-copy-link="/share/` + doneJobID + `"`, "Dental Smiles"} {
		if !strings.Contains(body, want) {
			t.Fatalf("preview missing %q", want)
		}
	}
}

func TestBuildLeadsRowLimit(t *testing.T) {
	csv := "title\nA\nB\nC\n"

	table, err := buildLeads(strings.NewReader(csv), 2)
	if err != nil {
		t.Fatal(err)
	}

	if len(table.Rows) != 2 || table.Total != 3 || !table.Truncated() {
		t.Fatalf("got %d rows, total %d, truncated %v", len(table.Rows), table.Total, table.Truncated())
	}
}

func TestDisplayURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://www.acme.com/":            "acme.com",
		"http://acme.com/locations/austin": "acme.com/locations/austin",
		"not a url":                        "not a url",
	} {
		if got := displayURL(in); got != want {
			t.Fatalf("displayURL(%q) = %q, want %q", in, got, want)
		}
	}
}
