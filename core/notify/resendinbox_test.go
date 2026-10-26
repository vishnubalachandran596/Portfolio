package notify

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/resend/resend-go/v4"
)

func TestInboxAPIDisabledMakesNoRequest(t *testing.T) {
	api := NewInboxAPI("")
	if api.Enabled() {
		t.Fatal("empty key should disable the client")
	}
	_, err := api.ListInboxes(context.Background(), nil)
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("got %v", err)
	}
}

func TestInboxAPIOperations(t *testing.T) {
	var mu sync.Mutex
	var gotMethod, gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotMethod, gotPath, gotAuth, gotBody = r.Method, r.URL.RequestURI(), r.Header.Get("Authorization"), string(body)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"object":"list","id":"ok","has_more":false,"data":[],"deleted":true}`)
	}))
	defer srv.Close()

	api := NewInboxAPIWithClient("test-key", srv.URL+"/", srv.Client())
	ctx := context.Background()
	limit := 5
	read := true
	html := "<p>note</p>"

	checks := []struct {
		name   string
		method string
		path   string
		body   string
		call   func() error
	}{
		{"list inboxes", http.MethodGet, "/inboxes", "", func() error {
			_, err := api.ListInboxes(ctx, &resend.ListOptions{Limit: &limit})
			return err
		}},
		{"get inbox", http.MethodGet, "/inboxes/inb", "", func() error {
			_, err := api.GetInbox(ctx, "inb")
			return err
		}},
		{"create inbox", http.MethodPost, "/inboxes", "support@example.com", func() error {
			_, err := api.CreateInbox(ctx, &resend.CreateInboxRequest{EmailAddress: "support@example.com", Name: "Support"})
			return err
		}},
		{"update inbox", http.MethodPatch, "/inboxes/inb", "Support", func() error {
			_, err := api.UpdateInbox(ctx, "inb", &resend.UpdateInboxRequest{Name: "Support"})
			return err
		}},
		{"delete inbox", http.MethodDelete, "/inboxes/inb", "", func() error {
			_, err := api.DeleteInbox(ctx, "inb")
			return err
		}},
		{"list labels", http.MethodGet, "/inboxes/inb/labels", "", func() error {
			_, err := api.ListLabels(ctx, "inb")
			return err
		}},
		{"create label", http.MethodPost, "/inboxes/inb/labels", "Orders", func() error {
			_, err := api.CreateLabel(ctx, "inb", &resend.CreateInboxLabelRequest{Name: "Orders", Color: "#112233"})
			return err
		}},
		{"update label", http.MethodPatch, "/inboxes/inb/labels/lbl", "Billing", func() error {
			_, err := api.UpdateLabel(ctx, "inb", "lbl", &resend.UpdateInboxLabelRequest{Name: "Billing"})
			return err
		}},
		{"delete label", http.MethodDelete, "/inboxes/inb/labels/lbl", "", func() error {
			_, err := api.DeleteLabel(ctx, "inb", "lbl")
			return err
		}},
		{"list threads", http.MethodGet, "/inboxes/inb/threads", "", func() error {
			_, err := api.ListThreads(ctx, "inb", &resend.ListInboxThreadsOptions{Folders: []resend.InboxFolder{resend.InboxFolderInbox}, Limit: &limit})
			return err
		}},
		{"search threads", http.MethodGet, "/inboxes/inb/threads/search", "", func() error {
			_, err := api.SearchThreads(ctx, "inb", &resend.SearchInboxThreadsOptions{Query: "invoice"})
			return err
		}},
		{"get thread", http.MethodGet, "/inboxes/inb/threads/thr", "", func() error {
			_, err := api.GetThread(ctx, "inb", "thr")
			return err
		}},
		{"update thread", http.MethodPatch, "/inboxes/inb/threads/thr", "archive", func() error {
			_, err := api.UpdateThread(ctx, "inb", "thr", &resend.UpdateInboxThreadRequest{Read: &read, Folder: resend.InboxMoveThreadFolderArchive})
			return err
		}},
		{"delete thread", http.MethodDelete, "/inboxes/inb/threads/thr", "", func() error {
			_, err := api.DeleteThread(ctx, "inb", "thr")
			return err
		}},
		{"add thread labels", http.MethodPost, "/inboxes/inb/threads/thr/labels", "lbl", func() error {
			_, err := api.AddThreadLabels(ctx, "inb", "thr", &resend.AddInboxThreadLabelsRequest{Labels: []string{"lbl"}})
			return err
		}},
		{"set thread labels", http.MethodPut, "/inboxes/inb/threads/thr/labels", "lbl", func() error {
			_, err := api.SetThreadLabels(ctx, "inb", "thr", &resend.SetInboxThreadLabelsRequest{Labels: []string{"lbl"}})
			return err
		}},
		{"remove thread label", http.MethodDelete, "/inboxes/inb/threads/thr/labels/lbl", "", func() error {
			_, err := api.RemoveThreadLabel(ctx, "inb", "thr", "lbl")
			return err
		}},
		{"list thread emails", http.MethodGet, "/inboxes/inb/threads/thr/emails", "", func() error {
			_, err := api.ListThreadEmails(ctx, "inb", "thr", &resend.ListOptions{Limit: &limit})
			return err
		}},
		{"get thread email", http.MethodGet, "/inboxes/inb/threads/thr/emails/eml", "", func() error {
			_, err := api.GetThreadEmail(ctx, "inb", "thr", "eml")
			return err
		}},
		{"reply", http.MethodPost, "/inboxes/inb/threads/thr/emails/eml/reply", "Thanks", func() error {
			_, err := api.ReplyThreadEmail(ctx, "inb", "thr", "eml", &resend.ReplyInboxThreadEmailRequest{Text: "Thanks", ReplyAll: true})
			return err
		}},
		{"forward", http.MethodPost, "/inboxes/inb/threads/thr/emails/eml/forward", "ada@example.com", func() error {
			_, err := api.ForwardThreadEmail(ctx, "inb", "thr", "eml", &resend.ForwardInboxThreadEmailRequest{To: []string{"ada@example.com"}, Text: "FYI"})
			return err
		}},
		{"list drafts", http.MethodGet, "/inboxes/inb/drafts", "", func() error {
			_, err := api.ListDrafts(ctx, "inb", &resend.ListOptions{Limit: &limit})
			return err
		}},
		{"get draft", http.MethodGet, "/inboxes/inb/drafts/drf", "", func() error {
			_, err := api.GetDraft(ctx, "inb", "drf")
			return err
		}},
		{"create draft", http.MethodPost, "/inboxes/inb/drafts", "Hello", func() error {
			_, err := api.CreateDraft(ctx, "inb", &resend.CreateInboxDraftRequest{To: []string{"ada@example.com"}, Subject: "Hello", Text: "Draft"})
			return err
		}},
		{"update draft", http.MethodPatch, "/inboxes/inb/drafts/drf", "Revised", func() error {
			_, err := api.UpdateDraft(ctx, "inb", "drf", &resend.UpdateInboxDraftRequest{Html: &html, Text: strPtr("Revised")})
			return err
		}},
		{"delete draft", http.MethodDelete, "/inboxes/inb/drafts/drf", "", func() error {
			_, err := api.DeleteDraft(ctx, "inb", "drf")
			return err
		}},
		{"send draft", http.MethodPost, "/inboxes/inb/drafts/drf/send", "", func() error {
			_, err := api.SendDraft(ctx, "inb", "drf", &resend.SendInboxDraftRequest{})
			return err
		}},
		{"get agent", http.MethodGet, "/inboxes/inb/agent", "", func() error {
			_, err := api.GetAgent(ctx, "inb")
			return err
		}},
		{"update agent", http.MethodPatch, "/inboxes/inb/agent", "concise_professional", func() error {
			tone := resend.InboxAgentToneConciseProfessional
			_, err := api.UpdateAgent(ctx, "inb", &resend.UpdateInboxAgentRequest{Tone: &tone, Actions: []resend.InboxAgentAction{resend.InboxAgentActionDraftReply}})
			return err
		}},
		{"list received", http.MethodGet, "/emails/receiving", "", func() error {
			_, err := api.ListReceived(ctx, &resend.ListOptions{Limit: &limit})
			return err
		}},
		{"get received", http.MethodGet, "/emails/receiving/eml", "", func() error {
			_, err := api.GetReceived(ctx, "eml")
			return err
		}},
		{"list attachments", http.MethodGet, "/emails/receiving/eml/attachments", "", func() error {
			_, err := api.ListAttachments(ctx, "eml", &resend.ListOptions{Limit: &limit})
			return err
		}},
		{"get attachment", http.MethodGet, "/emails/receiving/eml/attachments/att", "", func() error {
			_, err := api.GetAttachment(ctx, "eml", "att")
			return err
		}},
	}

	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			method, path, auth, body := gotMethod, gotPath, gotAuth, gotBody
			mu.Unlock()
			if method != tc.method {
				t.Fatalf("method %s", method)
			}
			if !strings.HasPrefix(path, tc.path) {
				t.Fatalf("path %s", path)
			}
			if auth != "Bearer test-key" {
				t.Fatalf("auth %q", auth)
			}
			if tc.body != "" && !strings.Contains(body, tc.body) {
				t.Fatalf("body %s", body)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
