package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The run endpoint must refuse anything that isn't the reader's own
// same-origin fetch before it looks anything up: these guards stop another
// site's script from running code (and spending the JDoodle plan) with a
// buyer's cookies.
func TestCourseRunRejectsForeignRequests(t *testing.T) {
	cases := map[string]http.Header{
		"no custom header":  {},
		"cross-site fetch":  {"X-Requested-With": {"vb-compiler"}, "Sec-Fetch-Site": {"cross-site"}},
		"same-site subhost": {"X-Requested-With": {"vb-compiler"}, "Sec-Fetch-Site": {"same-site"}},
	}
	for name, h := range cases {
		r := httptest.NewRequest(http.MethodPost, "/courses/x/learn/run", strings.NewReader(`{"lang":"java","code":"x"}`))
		r.Header = h
		w := httptest.NewRecorder()
		(&App{}).CourseRun(w, r)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", name, w.Code)
		}
		if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
			t.Errorf("%s: Cache-Control %q, want no-store", name, cc)
		}
	}
}

func TestCompilerOptionsCarryNoKeys(t *testing.T) {
	opts := compilerOptions(map[string]string{"java": "secret-java", "bash": "secret-bash"})
	if len(opts) != 2 || opts[0].Lang != "java" || opts[1].Lang != "bash" || opts[1].Label != "Bash" {
		t.Fatalf("unexpected options %#v", opts)
	}
}
