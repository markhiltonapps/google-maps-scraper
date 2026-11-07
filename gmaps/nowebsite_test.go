package gmaps_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/gosom/scrapemate"
	"github.com/stretchr/testify/require"

	"github.com/gosom/google-maps-scraper/exiter"
	"github.com/gosom/google-maps-scraper/gmaps"
)

func TestIsOwnWebsiteURL(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"":                               false,
		"   ":                            false,
		"https://www.acme-plumbing.com/": true,
		"http://acme.net/services?utm_source=gbp": true,
		"acme.com":                                   true,
		"https://www.facebook.com/acmeplumbing":      false,
		"https://m.facebook.com/acme":                false,
		"https://l.instagram.com/?u=x":               false,
		"https://x.com/acme":                         false,
		"https://www.yelp.com/biz/acme":              false,
		"https://linktr.ee/acme":                     false,
		"/url?q=https://www.facebook.com/acme&opi=1": false,
		"/url?q=https://acme.com/&opi=1":             true,
		// A domain that merely contains a social name is a real website.
		"https://notfacebook.com":           true,
		"https://facebook.com.acme.example": true,
	}

	for raw, want := range tests {
		require.Equal(t, want, gmaps.IsOwnWebsiteURL(raw), raw)
	}
}

func placeJSON(t *testing.T, website string) []byte {
	t.Helper()

	raw, err := os.ReadFile("../testdata/raw.json")
	require.NoError(t, err)

	var jd []any
	require.NoError(t, json.Unmarshal(raw, &jd))

	darray, ok := jd[6].([]any)
	require.True(t, ok)

	if website == "" {
		darray[7] = nil
	} else {
		darray[7] = []any{website}
	}

	out, err := json.Marshal(jd)
	require.NoError(t, err)

	return out
}

func processPlace(t *testing.T, website string, noWebsiteOnly bool) (job *gmaps.PlaceJob, result any, monitor *countingExiter) {
	t.Helper()

	monitor = &countingExiter{}
	opts := []gmaps.PlaceJobOptions{gmaps.WithPlaceJobExitMonitor(monitor)}

	if noWebsiteOnly {
		opts = append(opts, gmaps.WithPlaceJobNoWebsiteOnly())
	}

	job = gmaps.NewPlaceJob("input-1", "en", "https://www.google.com/maps/place/a", false, false, opts...)

	result, next, err := job.Process(context.Background(), &scrapemate.Response{
		Meta: map[string]any{"json": placeJSON(t, website)},
	})
	require.NoError(t, err)
	require.Empty(t, next)

	return job, result, monitor
}

// countingExiter records completed places; embedding makes the other methods no-ops.
type countingExiter struct {
	exiter.Exiter
	completed int
}

func (c *countingExiter) IncrPlacesCompleted(n int) { c.completed += n }

func TestPlaceJobNoWebsiteOnlyKeepsBusinessesWithoutWebsite(t *testing.T) {
	t.Parallel()

	for _, website := range []string{"", "https://www.facebook.com/acmeplumbing"} {
		job, result, monitor := processPlace(t, website, true)

		require.True(t, job.UseInResults(), "website %q must be kept", website)
		require.NotNil(t, result)
		require.Equal(t, 1, monitor.completed)
	}
}

func TestPlaceJobNoWebsiteOnlyDropsBusinessesWithWebsite(t *testing.T) {
	t.Parallel()

	job, result, monitor := processPlace(t, "https://www.acme-plumbing.com/", true)

	require.False(t, job.UseInResults())
	require.Nil(t, result)
	require.Equal(t, 1, monitor.completed, "a dropped place still counts as completed so the run can finish")
}

func TestPlaceJobWithoutFilterKeepsEverything(t *testing.T) {
	t.Parallel()

	job, result, _ := processPlace(t, "https://www.acme-plumbing.com/", false)

	require.True(t, job.UseInResults())
	require.NotNil(t, result)
}
