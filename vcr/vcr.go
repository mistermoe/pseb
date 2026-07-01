package vcr

import (
	"log"
	"net/http"
	"net/url"
	"testing"

	"github.com/alecthomas/assert/v2"
	libcassette "gopkg.in/dnaeon/go-vcr.v3/cassette"
	"gopkg.in/dnaeon/go-vcr.v3/recorder"
)

// Mode is used to represent the supported modes.
type Mode string

const (
	Replay Mode = "REPLAY"
	Record Mode = "RECORD"
)

func (m Mode) ToLibMode() recorder.Mode {
	switch m {
	case Record:
		return recorder.ModeRecordOnly
	case Replay:
		return recorder.ModeReplayOnly
	default:
		return recorder.ModeReplayOnly
	}
}

// Cassette can be used to retrieve the cassette that was recorded or replayed during the test.
type Cassette func() *libcassette.Cassette

func getCasette(r *recorder.Recorder, path string) Cassette {
	return func() *libcassette.Cassette {
		_ = r.Stop() // have to call Stop() to flush the cassette to disk.
		c, err := libcassette.Load(path)
		if err != nil {
			log.Fatalf("Failed to load cassette: %v", err)
		}

		return c
	}
}

// BootstrapFunc is a function that is used create and return the http client that will be used in the test.
// Additionally, the recorder can be used to further configure recording behavior as needed. More information
// on addition configuration can be found [here]
//
// [here]: https://github.com/dnaeon/go-vcr
type BootstrapFunc[T any] func(t *testing.T, m Mode, r *recorder.Recorder) T

// TestFunc is a function that contains the actual test logic. It should accept the client and the cassette as arguments.
type TestFunc[T any] func(*testing.T, T, Cassette)

// Test is a helper function that should be used when writing tests that make HTTP requests. It sets up everything necessary
// to record and replay HTTP interactions. The mode determines whether the test should record or replay interactions. Recorded
// interactions (a.k.a cassettes) are stored in `fixtures/${cassetteName}` adjacent to the test file that uses this function.
// A bootstrap function can be provided to set up the client that will be used in the test. The test function
// should contain the actual test logic. The cassette is passed to the test function so that it can be used to assert
// on the recorded interactions.
func Test[T any](t *testing.T, m Mode, bootstrap BootstrapFunc[T], test TestFunc[T]) {
	cassettePath := "fixtures/" + t.Name()
	rec, err := recorder.NewWithOptions(&recorder.Options{
		CassetteName: cassettePath,
		Mode:         m.ToLibMode(),
	})

	assert.NoError(t, err, "Failed to create recorder")

	defer rec.Stop() //nolint:errcheck

	// set up a hook to remove the Authorization header in recorded response
	hook := func(i *libcassette.Interaction) error {
		delete(i.Request.Headers, "Authorization")
		delete(i.Request.Headers, "Api-Key")
		delete(i.Request.Headers, "X-Partner-Name")
		return nil
	}

	rec.AddHook(hook, recorder.AfterCaptureHook)

	matcher := func(r *http.Request, cr libcassette.Request) bool {
		crurl, err := url.Parse(cr.URL)
		assert.NoError(t, err, "Failed to parse cassette URL")

		return r.Method == cr.Method && r.URL.Path == crurl.Path
	}

	rec.SetMatcher(matcher)

	client := bootstrap(t, m, rec)

	test(t, client, getCasette(rec, cassettePath))
}
