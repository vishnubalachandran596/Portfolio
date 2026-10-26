package notify

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/resend/resend-go/v4"
)

// InboxAPI is the Resend Inboxes client. Message bodies stay at Resend.
// An empty API key leaves the client disabled; it does not call the network.
type InboxAPI struct {
	client *resend.Client
}

// NewInboxAPI uses RESEND_API_KEY. An empty key returns a disabled client.
func NewInboxAPI(apiKey string) *InboxAPI {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return &InboxAPI{}
	}
	return &InboxAPI{client: resend.NewClient(key)}
}

// NewInboxAPIWithClient points the official SDK at a caller-supplied HTTP
// client and base URL. Tests use it against a fake server.
func NewInboxAPIWithClient(apiKey, baseURL string, httpClient *http.Client) *InboxAPI {
	api := NewInboxAPI(apiKey)
	if api.client == nil || httpClient == nil {
		return api
	}
	api.client = resend.NewCustomClient(httpClient, strings.TrimSpace(apiKey))
	if u, err := url.Parse(baseURL); err == nil {
		api.client.BaseURL = u
	}
	return api
}

// Enabled reports whether a key was configured.
func (a *InboxAPI) Enabled() bool {
	return a != nil && a.client != nil
}

func (a *InboxAPI) ready() error {
	if !a.Enabled() {
		return ErrDisabled
	}
	return nil
}

func (a *InboxAPI) ListInboxes(ctx context.Context, opt *resend.ListOptions) (resend.ListInboxesResponse, error) {
	if err := a.ready(); err != nil {
		return resend.ListInboxesResponse{}, err
	}
	return a.client.Inboxes.ListWithContext(ctx, opt)
}

func (a *InboxAPI) GetInbox(ctx context.Context, idOrEmail string) (resend.Inbox, error) {
	if err := a.ready(); err != nil {
		return resend.Inbox{}, err
	}
	return a.client.Inboxes.GetWithContext(ctx, idOrEmail)
}

func (a *InboxAPI) CreateInbox(ctx context.Context, params *resend.CreateInboxRequest) (resend.CreateInboxResponse, error) {
	if err := a.ready(); err != nil {
		return resend.CreateInboxResponse{}, err
	}
	return a.client.Inboxes.CreateWithContext(ctx, params)
}

func (a *InboxAPI) UpdateInbox(ctx context.Context, idOrEmail string, params *resend.UpdateInboxRequest) (resend.UpdateInboxResponse, error) {
	if err := a.ready(); err != nil {
		return resend.UpdateInboxResponse{}, err
	}
	return a.client.Inboxes.UpdateWithContext(ctx, idOrEmail, params)
}

func (a *InboxAPI) DeleteInbox(ctx context.Context, idOrEmail string) (resend.RemoveInboxResponse, error) {
	if err := a.ready(); err != nil {
		return resend.RemoveInboxResponse{}, err
	}
	return a.client.Inboxes.RemoveWithContext(ctx, idOrEmail)
}

func (a *InboxAPI) ListLabels(ctx context.Context, inboxID string) (resend.ListInboxLabelsResponse, error) {
	if err := a.ready(); err != nil {
		return resend.ListInboxLabelsResponse{}, err
	}
	return a.client.Inboxes.Labels.ListWithContext(ctx, inboxID)
}

func (a *InboxAPI) CreateLabel(ctx context.Context, inboxID string, params *resend.CreateInboxLabelRequest) (resend.CreateInboxLabelResponse, error) {
	if err := a.ready(); err != nil {
		return resend.CreateInboxLabelResponse{}, err
	}
	return a.client.Inboxes.Labels.CreateWithContext(ctx, inboxID, params)
}

func (a *InboxAPI) UpdateLabel(ctx context.Context, inboxID, labelID string, params *resend.UpdateInboxLabelRequest) (resend.UpdateInboxLabelResponse, error) {
	if err := a.ready(); err != nil {
		return resend.UpdateInboxLabelResponse{}, err
	}
	return a.client.Inboxes.Labels.UpdateWithContext(ctx, inboxID, labelID, params)
}

func (a *InboxAPI) DeleteLabel(ctx context.Context, inboxID, labelID string) (resend.RemoveInboxLabelResponse, error) {
	if err := a.ready(); err != nil {
		return resend.RemoveInboxLabelResponse{}, err
	}
	return a.client.Inboxes.Labels.RemoveWithContext(ctx, inboxID, labelID)
}

func (a *InboxAPI) ListThreads(ctx context.Context, inboxID string, opt *resend.ListInboxThreadsOptions) (resend.ListInboxThreadsResponse, error) {
	if err := a.ready(); err != nil {
		return resend.ListInboxThreadsResponse{}, err
	}
	return a.client.Inboxes.Threads.ListWithContext(ctx, inboxID, opt)
}

func (a *InboxAPI) SearchThreads(ctx context.Context, inboxID string, opt *resend.SearchInboxThreadsOptions) (resend.SearchInboxThreadsResponse, error) {
	if err := a.ready(); err != nil {
		return resend.SearchInboxThreadsResponse{}, err
	}
	return a.client.Inboxes.Threads.SearchWithContext(ctx, inboxID, opt)
}

func (a *InboxAPI) GetThread(ctx context.Context, inboxID, threadID string) (resend.InboxThreadSummary, error) {
	if err := a.ready(); err != nil {
		return resend.InboxThreadSummary{}, err
	}
	return a.client.Inboxes.Threads.GetWithContext(ctx, inboxID, threadID)
}

func (a *InboxAPI) UpdateThread(ctx context.Context, inboxID, threadID string, params *resend.UpdateInboxThreadRequest) (resend.InboxThreadSummary, error) {
	if err := a.ready(); err != nil {
		return resend.InboxThreadSummary{}, err
	}
	return a.client.Inboxes.Threads.UpdateWithContext(ctx, inboxID, threadID, params)
}

func (a *InboxAPI) DeleteThread(ctx context.Context, inboxID, threadID string) (resend.RemoveInboxThreadResponse, error) {
	if err := a.ready(); err != nil {
		return resend.RemoveInboxThreadResponse{}, err
	}
	return a.client.Inboxes.Threads.RemoveWithContext(ctx, inboxID, threadID)
}

func (a *InboxAPI) AddThreadLabels(ctx context.Context, inboxID, threadID string, params *resend.AddInboxThreadLabelsRequest) (resend.InboxThreadLabelsResponse, error) {
	if err := a.ready(); err != nil {
		return resend.InboxThreadLabelsResponse{}, err
	}
	return a.client.Inboxes.Threads.Labels.AddWithContext(ctx, inboxID, threadID, params)
}

func (a *InboxAPI) SetThreadLabels(ctx context.Context, inboxID, threadID string, params *resend.SetInboxThreadLabelsRequest) (resend.InboxThreadLabelsResponse, error) {
	if err := a.ready(); err != nil {
		return resend.InboxThreadLabelsResponse{}, err
	}
	return a.client.Inboxes.Threads.Labels.SetWithContext(ctx, inboxID, threadID, params)
}

func (a *InboxAPI) RemoveThreadLabel(ctx context.Context, inboxID, threadID, labelID string) (resend.InboxThreadLabelsResponse, error) {
	if err := a.ready(); err != nil {
		return resend.InboxThreadLabelsResponse{}, err
	}
	return a.client.Inboxes.Threads.Labels.RemoveWithContext(ctx, inboxID, threadID, labelID)
}

func (a *InboxAPI) ListThreadEmails(ctx context.Context, inboxID, threadID string, opt *resend.ListOptions) (resend.ListInboxThreadEmailsResponse, error) {
	if err := a.ready(); err != nil {
		return resend.ListInboxThreadEmailsResponse{}, err
	}
	return a.client.Inboxes.Threads.Emails.ListWithContext(ctx, inboxID, threadID, opt)
}

func (a *InboxAPI) GetThreadEmail(ctx context.Context, inboxID, threadID, emailID string) (resend.InboxThreadEmail, error) {
	if err := a.ready(); err != nil {
		return resend.InboxThreadEmail{}, err
	}
	return a.client.Inboxes.Threads.Emails.GetWithContext(ctx, inboxID, threadID, emailID)
}

func (a *InboxAPI) ReplyThreadEmail(ctx context.Context, inboxID, threadID, emailID string, params *resend.ReplyInboxThreadEmailRequest) (resend.ReplyInboxThreadEmailResponse, error) {
	if err := a.ready(); err != nil {
		return resend.ReplyInboxThreadEmailResponse{}, err
	}
	return a.client.Inboxes.Threads.Emails.ReplyWithContext(ctx, inboxID, threadID, emailID, params)
}

func (a *InboxAPI) ForwardThreadEmail(ctx context.Context, inboxID, threadID, emailID string, params *resend.ForwardInboxThreadEmailRequest) (resend.ForwardInboxThreadEmailResponse, error) {
	if err := a.ready(); err != nil {
		return resend.ForwardInboxThreadEmailResponse{}, err
	}
	return a.client.Inboxes.Threads.Emails.ForwardWithContext(ctx, inboxID, threadID, emailID, params)
}

func (a *InboxAPI) ListDrafts(ctx context.Context, inboxID string, opt *resend.ListOptions) (resend.ListInboxDraftsResponse, error) {
	if err := a.ready(); err != nil {
		return resend.ListInboxDraftsResponse{}, err
	}
	return a.client.Inboxes.Drafts.ListWithContext(ctx, inboxID, opt)
}

func (a *InboxAPI) GetDraft(ctx context.Context, inboxID, draftID string) (resend.InboxDraft, error) {
	if err := a.ready(); err != nil {
		return resend.InboxDraft{}, err
	}
	return a.client.Inboxes.Drafts.GetWithContext(ctx, inboxID, draftID)
}

func (a *InboxAPI) CreateDraft(ctx context.Context, inboxID string, params *resend.CreateInboxDraftRequest) (resend.CreateInboxDraftResponse, error) {
	if err := a.ready(); err != nil {
		return resend.CreateInboxDraftResponse{}, err
	}
	return a.client.Inboxes.Drafts.CreateWithContext(ctx, inboxID, params)
}

func (a *InboxAPI) UpdateDraft(ctx context.Context, inboxID, draftID string, params *resend.UpdateInboxDraftRequest) (resend.UpdateInboxDraftResponse, error) {
	if err := a.ready(); err != nil {
		return resend.UpdateInboxDraftResponse{}, err
	}
	return a.client.Inboxes.Drafts.UpdateWithContext(ctx, inboxID, draftID, params)
}

func (a *InboxAPI) DeleteDraft(ctx context.Context, inboxID, draftID string) (resend.RemoveInboxDraftResponse, error) {
	if err := a.ready(); err != nil {
		return resend.RemoveInboxDraftResponse{}, err
	}
	return a.client.Inboxes.Drafts.RemoveWithContext(ctx, inboxID, draftID)
}

func (a *InboxAPI) SendDraft(ctx context.Context, inboxID, draftID string, params *resend.SendInboxDraftRequest) (resend.SendInboxDraftResponse, error) {
	if err := a.ready(); err != nil {
		return resend.SendInboxDraftResponse{}, err
	}
	return a.client.Inboxes.Drafts.SendWithContext(ctx, inboxID, draftID, params)
}

func (a *InboxAPI) GetAgent(ctx context.Context, inboxID string) (resend.InboxAgent, error) {
	if err := a.ready(); err != nil {
		return resend.InboxAgent{}, err
	}
	return a.client.Inboxes.Agent.GetWithContext(ctx, inboxID)
}

func (a *InboxAPI) UpdateAgent(ctx context.Context, inboxID string, params *resend.UpdateInboxAgentRequest) (resend.UpdateInboxAgentResponse, error) {
	if err := a.ready(); err != nil {
		return resend.UpdateInboxAgentResponse{}, err
	}
	return a.client.Inboxes.Agent.UpdateWithContext(ctx, inboxID, params)
}

func (a *InboxAPI) ListReceived(ctx context.Context, opt *resend.ListOptions) (resend.ListReceivedEmailsResponse, error) {
	if err := a.ready(); err != nil {
		return resend.ListReceivedEmailsResponse{}, err
	}
	return a.client.Emails.Receiving.ListWithOptions(ctx, opt)
}

func (a *InboxAPI) GetReceived(ctx context.Context, emailID string) (*resend.ReceivedEmail, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.client.Emails.Receiving.GetWithContext(ctx, emailID)
}

func (a *InboxAPI) ListAttachments(ctx context.Context, emailID string, opt *resend.ListOptions) (resend.ListEmailAttachmentsResponse, error) {
	if err := a.ready(); err != nil {
		return resend.ListEmailAttachmentsResponse{}, err
	}
	return a.client.Emails.Receiving.ListAttachmentsWithOptions(ctx, emailID, opt)
}

func (a *InboxAPI) GetAttachment(ctx context.Context, emailID, attachmentID string) (*resend.EmailAttachment, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.client.Emails.Receiving.GetAttachmentWithContext(ctx, emailID, attachmentID)
}
