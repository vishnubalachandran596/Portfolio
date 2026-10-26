package handlers

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/resend/resend-go/v4"

	"github.com/vishnubalachandran596/portfolio/core/notify"
)

var (
	inboxRefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._%@+-]{0,199}$`)
	inboxIDPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,199}$`)
)

type inboxRow struct {
	ID, Name, Email, FromName, Last string
	Unread                          int
}

type inboxThreadRow struct {
	ID, Subject, From, To, When, Folder string
	Unread, HasAttachment, HasDraft     bool
	Count                               int
	Labels                              string
}

type inboxLabelRow struct {
	ID, Name, Color string
}

type inboxDraftRow struct {
	ID, Type, To, Subject, Updated string
}

type inboxAttachRow struct {
	ID, Filename, ContentType string
}

type inboxEmailRow struct {
	ID, Direction, From, To, Subject, When string
}

type AdminInboxListData struct {
	BaseData
	Enabled bool
	Inboxes []inboxRow
	HasMore bool
	After   string
	Notice  string
	Error   string
}

type AdminInboxData struct {
	BaseData
	Enabled    bool
	ID         string
	Name       string
	Email      string
	FromName   string
	Receiving  string
	Unread     int
	DraftCount int
	Folder     string
	Query      string
	Threads    []inboxThreadRow
	Labels     []inboxLabelRow
	Drafts     []inboxDraftRow
	AgentTone  string
	AgentNotes string
	AgentActs  map[string]bool
	HasMore    bool
	After      string
	Notice     string
	Error      string
}

type AdminInboxThreadData struct {
	BaseData
	InboxID, ThreadID, Subject, Folder string
	Read                               bool
	Labels                             []inboxLabelRow
	AllLabels                          []inboxLabelRow
	Emails                             []inboxEmailRow
	Notice, Error                      string
}

type AdminInboxEmailData struct {
	BaseData
	InboxID, ThreadID, EmailID   string
	Direction, From, To, Cc, Bcc string
	Subject, Text, HTML, When    string
	CanReply                     bool
	Attachments                  []inboxAttachRow
	Notice, Error                string
}

type AdminInboxDraftData struct {
	BaseData
	InboxID, DraftID, Type, To, Cc, Bcc, Subject, HTML, Text string
	Notice, Error                                            string
}

type AdminInboxReceivedData struct {
	BaseData
	Emails        []inboxEmailRow
	HasMore       bool
	After         string
	Notice, Error string
}

func (a *App) AdminInboxList(w http.ResponseWriter, r *http.Request) {
	data := AdminInboxListData{
		BaseData: a.base(r, "Inbox — Admin", "", "admin"),
		Enabled:  a.inboxOn(),
		Notice:   r.URL.Query().Get("notice"),
		Error:    r.URL.Query().Get("error"),
	}
	if data.Enabled {
		opt := inboxListOpt(r)
		list, err := a.Inbox.ListInboxes(r.Context(), opt)
		if err != nil {
			data.Error = inboxPublicErr(err)
		} else {
			data.HasMore = list.HasMore
			for _, item := range list.Data {
				data.Inboxes = append(data.Inboxes, inboxRow{
					ID: item.Id, Name: strp(item.Name), Email: item.EmailAddress,
					FromName: strp(item.FromName), Last: strp(item.LastReceived), Unread: item.Unread,
				})
				data.After = item.Id
			}
		}
	}
	a.View.Render(w, http.StatusOK, "admin_inbox_list", data)
}

func (a *App) AdminInboxCreate(w http.ResponseWriter, r *http.Request) {
	if !a.inboxReady(w, r) {
		return
	}
	if err := parseInboxForm(r); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(r.FormValue("email_address"))
	if email == "" {
		a.inboxBack(w, r, "/admin/inbox", "", "An email address is required.")
		return
	}
	_, err := a.Inbox.CreateInbox(r.Context(), &resend.CreateInboxRequest{
		EmailAddress: email,
		Name:         strings.TrimSpace(r.FormValue("name")),
		FromName:     strings.TrimSpace(r.FormValue("from_name")),
		Forwarding:   r.FormValue("forwarding") == "1",
	})
	if err != nil {
		a.inboxBack(w, r, "/admin/inbox", "", inboxPublicErr(err))
		return
	}
	a.inboxBack(w, r, "/admin/inbox", "created", "")
}

func (a *App) AdminInboxShow(w http.ResponseWriter, r *http.Request) {
	id, ok := inboxRef(w, r, "id")
	if !ok {
		return
	}
	data := AdminInboxData{
		BaseData:  a.base(r, "Inbox — Admin", "", "admin"),
		Enabled:   a.inboxOn(),
		ID:        id,
		Folder:    r.URL.Query().Get("folder"),
		Query:     r.URL.Query().Get("q"),
		Notice:    r.URL.Query().Get("notice"),
		Error:     r.URL.Query().Get("error"),
		AgentActs: map[string]bool{},
	}
	if !data.Enabled {
		a.View.Render(w, http.StatusOK, "admin_inbox", data)
		return
	}
	box, err := a.Inbox.GetInbox(r.Context(), id)
	if err != nil {
		data.Error = inboxPublicErr(err)
		a.View.Render(w, http.StatusOK, "admin_inbox", data)
		return
	}
	data.Name = box.Name
	data.Email = box.EmailAddress
	data.FromName = strp(box.FromName)
	data.Receiving = strp(box.ReceivingAddress)
	data.Unread = box.Unread
	data.DraftCount = box.Drafts
	a.fillInboxLists(r, &data)
	a.View.Render(w, http.StatusOK, "admin_inbox", data)
}

func (a *App) fillInboxLists(r *http.Request, data *AdminInboxData) {
	if data.Query != "" {
		found, err := a.Inbox.SearchThreads(r.Context(), data.ID, &resend.SearchInboxThreadsOptions{
			Query: data.Query, Folders: inboxFolders(data.Folder), Limit: inboxLimit(r), After: inboxAfter(r),
		})
		if err != nil {
			data.Error = joinErr(data.Error, inboxPublicErr(err))
		} else {
			data.HasMore = found.HasMore
			for _, th := range found.Data {
				data.Threads = append(data.Threads, threadRow(th.InboxThread))
				data.After = th.Id
			}
		}
	} else {
		list, err := a.Inbox.ListThreads(r.Context(), data.ID, &resend.ListInboxThreadsOptions{
			Folders: inboxFolders(data.Folder), Limit: inboxLimit(r), After: inboxAfter(r),
		})
		if err != nil {
			data.Error = joinErr(data.Error, inboxPublicErr(err))
		} else {
			data.HasMore = list.HasMore
			for _, th := range list.Data {
				data.Threads = append(data.Threads, threadRow(th))
				data.After = th.Id
			}
		}
	}
	labels, err := a.Inbox.ListLabels(r.Context(), data.ID)
	if err != nil {
		data.Error = joinErr(data.Error, inboxPublicErr(err))
	} else {
		for _, lb := range labels.Data {
			data.Labels = append(data.Labels, inboxLabelRow{ID: lb.Id, Name: lb.Name, Color: lb.Color})
		}
	}
	drafts, err := a.Inbox.ListDrafts(r.Context(), data.ID, &resend.ListOptions{Limit: inboxLimit(r)})
	if err != nil {
		data.Error = joinErr(data.Error, inboxPublicErr(err))
	} else {
		for _, d := range drafts.Data {
			data.Drafts = append(data.Drafts, inboxDraftRow{
				ID: d.Id, Type: string(d.Type), To: strings.Join(d.To, ", "),
				Subject: strp(d.Subject), Updated: d.UpdatedAt,
			})
		}
	}
	agent, err := a.Inbox.GetAgent(r.Context(), data.ID)
	if err != nil {
		data.Error = joinErr(data.Error, inboxPublicErr(err))
	} else {
		data.AgentNotes = strp(agent.Instructions)
		data.AgentTone = strp(agent.Tone)
		for _, act := range agent.Actions {
			data.AgentActs[string(act)] = true
		}
	}
}

func (a *App) AdminInboxUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := inboxRef(w, r, "id")
	if !ok || !a.inboxReady(w, r) {
		return
	}
	if err := parseInboxForm(r); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	params := &resend.UpdateInboxRequest{Name: strings.TrimSpace(r.FormValue("name"))}
	if r.FormValue("clear_from_name") == "1" {
		empty := ""
		params.FromName = &empty
	} else if name := strings.TrimSpace(r.FormValue("from_name")); name != "" {
		params.FromName = &name
	}
	if _, err := a.Inbox.UpdateInbox(r.Context(), id, params); err != nil {
		a.inboxBack(w, r, "/admin/inbox/"+id, "", inboxPublicErr(err))
		return
	}
	a.inboxBack(w, r, "/admin/inbox/"+id, "updated", "")
}

func (a *App) AdminInboxDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := inboxRef(w, r, "id")
	if !ok || !a.inboxReady(w, r) {
		return
	}
	if _, err := a.Inbox.DeleteInbox(r.Context(), id); err != nil {
		a.inboxBack(w, r, "/admin/inbox/"+id, "", inboxPublicErr(err))
		return
	}
	a.inboxBack(w, r, "/admin/inbox", "deleted", "")
}

func (a *App) AdminInboxLabelCreate(w http.ResponseWriter, r *http.Request) {
	id, ok := inboxRef(w, r, "id")
	if !ok || !a.inboxReady(w, r) {
		return
	}
	if err := parseInboxForm(r); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if _, err := a.Inbox.CreateLabel(r.Context(), id, &resend.CreateInboxLabelRequest{
		Name: strings.TrimSpace(r.FormValue("name")), Color: strings.TrimSpace(r.FormValue("color")),
	}); err != nil {
		a.inboxBack(w, r, "/admin/inbox/"+id, "", inboxPublicErr(err))
		return
	}
	a.inboxBack(w, r, "/admin/inbox/"+id, "label", "")
}

func (a *App) AdminInboxLabelUpdate(w http.ResponseWriter, r *http.Request) {
	id, label, ok := inboxPair(w, r, "id", "label")
	if !ok || !a.inboxReady(w, r) {
		return
	}
	if err := parseInboxForm(r); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if _, err := a.Inbox.UpdateLabel(r.Context(), id, label, &resend.UpdateInboxLabelRequest{
		Name: strings.TrimSpace(r.FormValue("name")), Color: strings.TrimSpace(r.FormValue("color")),
	}); err != nil {
		a.inboxBack(w, r, "/admin/inbox/"+id, "", inboxPublicErr(err))
		return
	}
	a.inboxBack(w, r, "/admin/inbox/"+id, "label", "")
}

func (a *App) AdminInboxLabelDelete(w http.ResponseWriter, r *http.Request) {
	id, label, ok := inboxPair(w, r, "id", "label")
	if !ok || !a.inboxReady(w, r) {
		return
	}
	if _, err := a.Inbox.DeleteLabel(r.Context(), id, label); err != nil {
		a.inboxBack(w, r, "/admin/inbox/"+id, "", inboxPublicErr(err))
		return
	}
	a.inboxBack(w, r, "/admin/inbox/"+id, "label", "")
}

func (a *App) AdminInboxAgentUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := inboxRef(w, r, "id")
	if !ok || !a.inboxReady(w, r) {
		return
	}
	if err := parseInboxForm(r); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	params := &resend.UpdateInboxAgentRequest{}
	if r.FormValue("clear_instructions") == "1" {
		params.ClearInstructions()
	} else if notes := strings.TrimSpace(r.FormValue("instructions")); notes != "" {
		params.Instructions = &notes
	}
	if r.FormValue("clear_tone") == "1" {
		params.ClearTone()
	} else if tone := strings.TrimSpace(r.FormValue("tone")); tone != "" {
		t := resend.InboxAgentTone(tone)
		params.Tone = &t
	}
	params.Actions = []resend.InboxAgentAction{}
	for _, act := range r.Form["action"] {
		if act != "" {
			params.Actions = append(params.Actions, resend.InboxAgentAction(act))
		}
	}
	if _, err := a.Inbox.UpdateAgent(r.Context(), id, params); err != nil {
		a.inboxBack(w, r, "/admin/inbox/"+id, "", inboxPublicErr(err))
		return
	}
	a.inboxBack(w, r, "/admin/inbox/"+id, "agent", "")
}

func (a *App) AdminInboxThread(w http.ResponseWriter, r *http.Request) {
	id, thread, ok := inboxPair(w, r, "id", "thread")
	if !ok {
		return
	}
	data := AdminInboxThreadData{
		BaseData: a.base(r, "Thread — Admin", "", "admin"),
		InboxID:  id, ThreadID: thread,
		Notice: r.URL.Query().Get("notice"), Error: r.URL.Query().Get("error"),
	}
	if !a.inboxOn() {
		data.Error = inboxPublicErr(notify.ErrDisabled)
		a.View.Render(w, http.StatusOK, "admin_inbox_thread", data)
		return
	}
	sum, err := a.Inbox.GetThread(r.Context(), id, thread)
	if err != nil {
		data.Error = inboxPublicErr(err)
		a.View.Render(w, http.StatusOK, "admin_inbox_thread", data)
		return
	}
	data.Subject = strp(sum.Subject)
	data.Folder = string(sum.Folder)
	data.Read = sum.Read
	for _, lb := range sum.Labels {
		data.Labels = append(data.Labels, inboxLabelRow{ID: lb.Id, Name: lb.Name, Color: lb.Color})
	}
	if labels, lerr := a.Inbox.ListLabels(r.Context(), id); lerr == nil {
		for _, lb := range labels.Data {
			data.AllLabels = append(data.AllLabels, inboxLabelRow{ID: lb.Id, Name: lb.Name, Color: lb.Color})
		}
	}
	emails, err := a.Inbox.ListThreadEmails(r.Context(), id, thread, inboxListOpt(r))
	if err != nil {
		data.Error = joinErr(data.Error, inboxPublicErr(err))
	} else {
		for _, em := range emails.Data {
			data.Emails = append(data.Emails, inboxEmailRow{
				ID: em.Id, Direction: string(em.Direction), From: em.From,
				To: strings.Join(em.To, ", "), Subject: strp(em.Subject), When: em.ReceivedAt,
			})
		}
	}
	a.View.Render(w, http.StatusOK, "admin_inbox_thread", data)
}

func (a *App) AdminInboxThreadUpdate(w http.ResponseWriter, r *http.Request) {
	id, thread, ok := inboxPair(w, r, "id", "thread")
	if !ok || !a.inboxReady(w, r) {
		return
	}
	if err := parseInboxForm(r); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	read := r.FormValue("read") == "1"
	params := &resend.UpdateInboxThreadRequest{Read: &read}
	if folder := strings.TrimSpace(r.FormValue("folder")); folder != "" {
		params.Folder = resend.InboxMoveThreadFolder(folder)
	}
	if _, err := a.Inbox.UpdateThread(r.Context(), id, thread, params); err != nil {
		a.inboxBack(w, r, inboxThreadPath(id, thread), "", inboxPublicErr(err))
		return
	}
	a.inboxBack(w, r, inboxThreadPath(id, thread), "updated", "")
}

func (a *App) AdminInboxThreadDelete(w http.ResponseWriter, r *http.Request) {
	id, thread, ok := inboxPair(w, r, "id", "thread")
	if !ok || !a.inboxReady(w, r) {
		return
	}
	if _, err := a.Inbox.DeleteThread(r.Context(), id, thread); err != nil {
		a.inboxBack(w, r, inboxThreadPath(id, thread), "", inboxPublicErr(err))
		return
	}
	a.inboxBack(w, r, "/admin/inbox/"+id, "deleted", "")
}

func (a *App) AdminInboxThreadLabelsAdd(w http.ResponseWriter, r *http.Request) {
	a.inboxThreadLabels(w, r, "add")
}

func (a *App) AdminInboxThreadLabelsSet(w http.ResponseWriter, r *http.Request) {
	a.inboxThreadLabels(w, r, "set")
}

func (a *App) inboxThreadLabels(w http.ResponseWriter, r *http.Request, mode string) {
	id, thread, ok := inboxPair(w, r, "id", "thread")
	if !ok || !a.inboxReady(w, r) {
		return
	}
	if err := parseInboxForm(r); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	ids := r.Form["label_id"]
	var err error
	if mode == "set" {
		_, err = a.Inbox.SetThreadLabels(r.Context(), id, thread, &resend.SetInboxThreadLabelsRequest{Labels: ids})
	} else {
		_, err = a.Inbox.AddThreadLabels(r.Context(), id, thread, &resend.AddInboxThreadLabelsRequest{Labels: ids})
	}
	if err != nil {
		a.inboxBack(w, r, inboxThreadPath(id, thread), "", inboxPublicErr(err))
		return
	}
	a.inboxBack(w, r, inboxThreadPath(id, thread), "labels", "")
}

func (a *App) AdminInboxThreadLabelRemove(w http.ResponseWriter, r *http.Request) {
	id, thread, ok := inboxPair(w, r, "id", "thread")
	if !ok || !a.inboxReady(w, r) {
		return
	}
	label := r.PathValue("label")
	if !inboxIDPattern.MatchString(label) {
		a.NotFound(w, r)
		return
	}
	if _, err := a.Inbox.RemoveThreadLabel(r.Context(), id, thread, label); err != nil {
		a.inboxBack(w, r, inboxThreadPath(id, thread), "", inboxPublicErr(err))
		return
	}
	a.inboxBack(w, r, inboxThreadPath(id, thread), "labels", "")
}

func (a *App) AdminInboxEmail(w http.ResponseWriter, r *http.Request) {
	id, thread, ok := inboxPair(w, r, "id", "thread")
	if !ok {
		return
	}
	emailID := r.PathValue("email")
	if !inboxIDPattern.MatchString(emailID) {
		a.NotFound(w, r)
		return
	}
	data := AdminInboxEmailData{
		BaseData: a.base(r, "Message — Admin", "", "admin"),
		InboxID:  id, ThreadID: thread, EmailID: emailID, CanReply: true,
		Notice: r.URL.Query().Get("notice"), Error: r.URL.Query().Get("error"),
	}
	if !a.inboxOn() {
		data.Error = inboxPublicErr(notify.ErrDisabled)
		a.View.Render(w, http.StatusOK, "admin_inbox_email", data)
		return
	}
	em, err := a.Inbox.GetThreadEmail(r.Context(), id, thread, emailID)
	if err != nil {
		data.Error = inboxPublicErr(err)
		a.View.Render(w, http.StatusOK, "admin_inbox_email", data)
		return
	}
	data.Direction = string(em.Direction)
	data.From = em.From
	data.To = strings.Join(em.To, ", ")
	data.Cc = strings.Join(em.Cc, ", ")
	data.Bcc = strings.Join(em.Bcc, ", ")
	data.Subject = strp(em.Subject)
	data.Text = strp(em.Text)
	data.HTML = strp(em.Html)
	data.When = em.ReceivedAt
	for _, att := range em.Attachments {
		data.Attachments = append(data.Attachments, inboxAttachRow{ID: att.Id, Filename: strp(att.Filename)})
	}
	if listed, lerr := a.Inbox.ListAttachments(r.Context(), emailID, nil); lerr == nil {
		data.Attachments = nil
		for _, att := range listed.Data {
			data.Attachments = append(data.Attachments, inboxAttachRow{ID: att.Id, Filename: att.Filename, ContentType: att.ContentType})
		}
	}
	a.View.Render(w, http.StatusOK, "admin_inbox_email", data)
}

func (a *App) AdminInboxReply(w http.ResponseWriter, r *http.Request) {
	a.inboxSend(w, r, true)
}

func (a *App) AdminInboxForward(w http.ResponseWriter, r *http.Request) {
	a.inboxSend(w, r, false)
}

func (a *App) inboxSend(w http.ResponseWriter, r *http.Request, reply bool) {
	id, thread, ok := inboxPair(w, r, "id", "thread")
	if !ok || !a.inboxReady(w, r) {
		return
	}
	emailID := r.PathValue("email")
	if !inboxIDPattern.MatchString(emailID) {
		a.NotFound(w, r)
		return
	}
	if err := parseInboxForm(r); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	file, _, ferr := readInboxUpload(r)
	if ferr != nil {
		a.inboxBack(w, r, inboxEmailPath(id, thread, emailID), "", ferr.Error())
		return
	}
	var err error
	if reply {
		params := &resend.ReplyInboxThreadEmailRequest{
			Cc: splitList(r.FormValue("cc")), Bcc: splitList(r.FormValue("bcc")),
			Html: r.FormValue("html"), Text: r.FormValue("text"),
			Subject:  strings.TrimSpace(r.FormValue("subject")),
			ReplyAll: r.FormValue("reply_all") == "1",
		}
		if file != nil {
			params.Attachments = []*resend.Attachment{file}
		}
		_, err = a.Inbox.ReplyThreadEmail(r.Context(), id, thread, emailID, params)
	} else {
		params := &resend.ForwardInboxThreadEmailRequest{
			To: splitList(r.FormValue("to")), Cc: splitList(r.FormValue("cc")), Bcc: splitList(r.FormValue("bcc")),
			Html: r.FormValue("html"), Text: r.FormValue("text"),
			Subject: strings.TrimSpace(r.FormValue("subject")),
		}
		if file != nil {
			params.Attachments = []*resend.Attachment{file}
		}
		_, err = a.Inbox.ForwardThreadEmail(r.Context(), id, thread, emailID, params)
	}
	if err != nil {
		a.inboxBack(w, r, inboxEmailPath(id, thread, emailID), "", inboxPublicErr(err))
		return
	}
	a.inboxBack(w, r, inboxThreadPath(id, thread), "sent", "")
}

func (a *App) AdminInboxDraftCreate(w http.ResponseWriter, r *http.Request) {
	id, ok := inboxRef(w, r, "id")
	if !ok || !a.inboxReady(w, r) {
		return
	}
	if err := parseInboxForm(r); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	_, draftFile, ferr := readInboxUpload(r)
	if ferr != nil {
		a.inboxBack(w, r, "/admin/inbox/"+id, "", ferr.Error())
		return
	}
	params := &resend.CreateInboxDraftRequest{
		To: splitList(r.FormValue("to")), Cc: splitList(r.FormValue("cc")), Bcc: splitList(r.FormValue("bcc")),
		Subject: strings.TrimSpace(r.FormValue("subject")), Html: r.FormValue("html"), Text: r.FormValue("text"),
		ThreadId: strings.TrimSpace(r.FormValue("thread_id")), ReplyToEmailId: strings.TrimSpace(r.FormValue("reply_to_email_id")),
	}
	if draftFile != nil {
		params.Attachments = []*resend.InboxDraftAttachmentInput{draftFile}
	}
	created, err := a.Inbox.CreateDraft(r.Context(), id, params)
	if err != nil {
		a.inboxBack(w, r, "/admin/inbox/"+id, "", inboxPublicErr(err))
		return
	}
	a.inboxBack(w, r, "/admin/inbox/"+id+"/drafts/"+url.PathEscape(created.Id), "draft", "")
}

func (a *App) AdminInboxDraft(w http.ResponseWriter, r *http.Request) {
	id, draft, ok := inboxPair(w, r, "id", "draft")
	if !ok {
		return
	}
	data := AdminInboxDraftData{
		BaseData: a.base(r, "Draft — Admin", "", "admin"),
		InboxID:  id, DraftID: draft,
		Notice: r.URL.Query().Get("notice"), Error: r.URL.Query().Get("error"),
	}
	if !a.inboxOn() {
		data.Error = inboxPublicErr(notify.ErrDisabled)
		a.View.Render(w, http.StatusOK, "admin_inbox_draft", data)
		return
	}
	row, err := a.Inbox.GetDraft(r.Context(), id, draft)
	if err != nil {
		data.Error = inboxPublicErr(err)
		a.View.Render(w, http.StatusOK, "admin_inbox_draft", data)
		return
	}
	data.Type = string(row.Type)
	data.To = strings.Join(row.To, ", ")
	data.Cc = strings.Join(row.Cc, ", ")
	data.Bcc = strings.Join(row.Bcc, ", ")
	data.Subject = strp(row.Subject)
	data.HTML = strp(row.Html)
	data.Text = strp(row.Text)
	a.View.Render(w, http.StatusOK, "admin_inbox_draft", data)
}

func (a *App) AdminInboxDraftUpdate(w http.ResponseWriter, r *http.Request) {
	id, draft, ok := inboxPair(w, r, "id", "draft")
	if !ok || !a.inboxReady(w, r) {
		return
	}
	if err := parseInboxForm(r); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	htmlBody := r.FormValue("html")
	textBody := r.FormValue("text")
	subject := strings.TrimSpace(r.FormValue("subject"))
	params := &resend.UpdateInboxDraftRequest{
		To: splitList(r.FormValue("to")), Cc: splitList(r.FormValue("cc")), Bcc: splitList(r.FormValue("bcc")),
		Subject: &subject, Html: &htmlBody, Text: &textBody,
	}
	if _, err := a.Inbox.UpdateDraft(r.Context(), id, draft, params); err != nil {
		a.inboxBack(w, r, "/admin/inbox/"+id+"/drafts/"+draft, "", inboxPublicErr(err))
		return
	}
	a.inboxBack(w, r, "/admin/inbox/"+id+"/drafts/"+draft, "saved", "")
}

func (a *App) AdminInboxDraftDelete(w http.ResponseWriter, r *http.Request) {
	id, draft, ok := inboxPair(w, r, "id", "draft")
	if !ok || !a.inboxReady(w, r) {
		return
	}
	if _, err := a.Inbox.DeleteDraft(r.Context(), id, draft); err != nil {
		a.inboxBack(w, r, "/admin/inbox/"+id+"/drafts/"+draft, "", inboxPublicErr(err))
		return
	}
	a.inboxBack(w, r, "/admin/inbox/"+id, "deleted", "")
}

func (a *App) AdminInboxDraftSend(w http.ResponseWriter, r *http.Request) {
	id, draft, ok := inboxPair(w, r, "id", "draft")
	if !ok || !a.inboxReady(w, r) {
		return
	}
	if _, err := a.Inbox.SendDraft(r.Context(), id, draft, &resend.SendInboxDraftRequest{}); err != nil {
		a.inboxBack(w, r, "/admin/inbox/"+id+"/drafts/"+draft, "", inboxPublicErr(err))
		return
	}
	a.inboxBack(w, r, "/admin/inbox/"+id, "sent", "")
}

func (a *App) AdminInboxReceived(w http.ResponseWriter, r *http.Request) {
	data := AdminInboxReceivedData{
		BaseData: a.base(r, "Received mail — Admin", "", "admin"),
		Notice:   r.URL.Query().Get("notice"),
		Error:    r.URL.Query().Get("error"),
	}
	if !a.inboxOn() {
		data.Error = inboxPublicErr(notify.ErrDisabled)
		a.View.Render(w, http.StatusOK, "admin_inbox_received", data)
		return
	}
	list, err := a.Inbox.ListReceived(r.Context(), inboxListOpt(r))
	if err != nil {
		data.Error = inboxPublicErr(err)
	} else {
		data.HasMore = list.HasMore
		for _, em := range list.Data {
			data.Emails = append(data.Emails, inboxEmailRow{
				ID: em.Id, From: em.From, To: strings.Join(em.To, ", "),
				Subject: em.Subject, When: em.CreatedAt,
			})
			data.After = em.Id
		}
	}
	a.View.Render(w, http.StatusOK, "admin_inbox_received", data)
}

func (a *App) AdminInboxReceivedEmail(w http.ResponseWriter, r *http.Request) {
	emailID := r.PathValue("email")
	if !inboxIDPattern.MatchString(emailID) {
		a.NotFound(w, r)
		return
	}
	data := AdminInboxEmailData{
		BaseData: a.base(r, "Received message — Admin", "", "admin"),
		EmailID:  emailID,
		Notice:   r.URL.Query().Get("notice"),
		Error:    r.URL.Query().Get("error"),
	}
	if !a.inboxOn() {
		data.Error = inboxPublicErr(notify.ErrDisabled)
		a.View.Render(w, http.StatusOK, "admin_inbox_email", data)
		return
	}
	em, err := a.Inbox.GetReceived(r.Context(), emailID)
	if err != nil {
		data.Error = inboxPublicErr(err)
		a.View.Render(w, http.StatusOK, "admin_inbox_email", data)
		return
	}
	data.From = em.From
	data.To = strings.Join(em.To, ", ")
	data.Cc = strings.Join(em.Cc, ", ")
	data.Bcc = strings.Join(em.Bcc, ", ")
	data.Subject = em.Subject
	data.Text = em.Text
	data.HTML = em.Html
	data.When = em.CreatedAt
	for _, att := range em.Attachments {
		data.Attachments = append(data.Attachments, inboxAttachRow{ID: att.Id, Filename: att.Filename, ContentType: att.ContentType})
	}
	a.View.Render(w, http.StatusOK, "admin_inbox_email", data)
}

func (a *App) AdminInboxAttachment(w http.ResponseWriter, r *http.Request) {
	emailID := r.PathValue("email")
	attachmentID := r.PathValue("attachment")
	if !inboxIDPattern.MatchString(emailID) || !inboxIDPattern.MatchString(attachmentID) {
		a.NotFound(w, r)
		return
	}
	if !a.inboxOn() {
		http.Error(w, inboxPublicErr(notify.ErrDisabled), http.StatusServiceUnavailable)
		return
	}
	att, err := a.Inbox.GetAttachment(r.Context(), emailID, attachmentID)
	if err != nil {
		http.Error(w, inboxPublicErr(err), http.StatusBadGateway)
		return
	}
	if att == nil || !strings.HasPrefix(att.DownloadUrl, "https://") {
		http.Error(w, "attachment unavailable", http.StatusBadGateway)
		return
	}
	http.Redirect(w, r, att.DownloadUrl, http.StatusSeeOther)
}

func (a *App) inboxOn() bool { return a.Inbox != nil && a.Inbox.Enabled() }

func (a *App) inboxReady(w http.ResponseWriter, r *http.Request) bool {
	if a.inboxOn() {
		return true
	}
	a.inboxBack(w, r, "/admin/inbox", "", inboxPublicErr(notify.ErrDisabled))
	return false
}

func (a *App) inboxBack(w http.ResponseWriter, r *http.Request, path, notice, errMsg string) {
	q := url.Values{}
	if notice != "" {
		q.Set("notice", notice)
	}
	if errMsg != "" {
		q.Set("error", errMsg)
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	http.Redirect(w, r, path, http.StatusSeeOther)
}

func inboxRef(w http.ResponseWriter, r *http.Request, name string) (string, bool) {
	id := r.PathValue(name)
	if !inboxRefPattern.MatchString(id) {
		http.NotFound(w, r)
		return "", false
	}
	return id, true
}

func inboxPair(w http.ResponseWriter, r *http.Request, aName, bName string) (string, string, bool) {
	a, ok := inboxRef(w, r, aName)
	if !ok {
		return "", "", false
	}
	b := r.PathValue(bName)
	if !inboxIDPattern.MatchString(b) {
		http.NotFound(w, r)
		return "", "", false
	}
	return a, b, true
}

func parseInboxForm(r *http.Request) error {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		return r.ParseMultipartForm(8 << 20)
	}
	return r.ParseForm()
}

func readInboxUpload(r *http.Request) (*resend.Attachment, *resend.InboxDraftAttachmentInput, error) {
	if r.MultipartForm == nil {
		return nil, nil, nil
	}
	f, hdr, err := r.FormFile("attachment")
	if err != nil {
		return nil, nil, nil
	}
	defer f.Close()
	buf, err := io.ReadAll(io.LimitReader(f, (8<<20)+1))
	if err != nil {
		return nil, nil, err
	}
	if len(buf) > 8<<20 {
		return nil, nil, errors.New("attachment is larger than 8 MB")
	}
	if len(buf) == 0 {
		return nil, nil, errors.New("attachment is empty")
	}
	ct := hdr.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	return &resend.Attachment{Content: buf, Filename: hdr.Filename, ContentType: ct},
		&resend.InboxDraftAttachmentInput{Content: buf, Filename: hdr.Filename, ContentType: ct}, nil
}

func splitList(s string) []string {
	out := []string{}
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' }) {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func inboxListOpt(r *http.Request) *resend.ListOptions {
	return &resend.ListOptions{Limit: inboxLimit(r), After: inboxAfter(r)}
}

func inboxLimit(r *http.Request) *int {
	n := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 && v <= 100 {
			n = v
		}
	}
	return &n
}

func inboxAfter(r *http.Request) *string {
	if after := r.URL.Query().Get("after"); after != "" && inboxIDPattern.MatchString(after) {
		return &after
	}
	return nil
}

func inboxFolders(folder string) []resend.InboxFolder {
	switch folder {
	case "archive", "spam", "trash", "sent", "inbox":
		return []resend.InboxFolder{folder}
	default:
		return nil
	}
}

func threadRow(th resend.InboxThread) inboxThreadRow {
	var labels []string
	for _, lb := range th.Labels {
		labels = append(labels, lb.Name)
	}
	return inboxThreadRow{
		ID: th.Id, Subject: strp(th.Subject), From: strp(th.From), To: strings.Join(th.To, ", "),
		When: th.ReceivedAt, Folder: string(th.Folder), Unread: !th.Read,
		HasAttachment: th.HasAttachment, HasDraft: th.HasDraft, Count: th.EmailCount,
		Labels: strings.Join(labels, ", "),
	}
}

func inboxThreadPath(inboxID, threadID string) string {
	return "/admin/inbox/" + inboxID + "/threads/" + threadID
}

func inboxEmailPath(inboxID, threadID, emailID string) string {
	return inboxThreadPath(inboxID, threadID) + "/emails/" + emailID
}

func strp[T ~string](s *T) string {
	if s == nil {
		return ""
	}
	return string(*s)
}

func joinErr(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" || strings.Contains(a, b) {
		return a
	}
	return a + " " + b
}

func inboxPublicErr(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, notify.ErrDisabled) {
		return "Resend is off until its API key is set."
	}
	s := strings.Join(strings.Fields(err.Error()), " ")
	if i := strings.Index(s, "re_"); i >= 0 {
		s = s[:i] + "[redacted]"
	}
	s = strings.ReplaceAll(s, "Bearer ", "Bearer [redacted] ")
	if len(s) > 180 {
		s = s[:180]
	}
	return s
}
