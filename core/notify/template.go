package notify

import (
	"bytes"
	"fmt"
	"html/template"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/vishnubalachandran596/portfolio/core/models"
	texttemplate "text/template"
	"text/template/parse"
)

// MaxTemplateBytes is the admin upload limit for one HTML or text body.
const MaxTemplateBytes = 200 * 1024

var (
	forbiddenBlocks = regexp.MustCompile(`(?i)\{\{-?\s*(define|template|block)\b`)
	forbiddenTags   = regexp.MustCompile(`(?i)<\s*(script|iframe|form|object|embed)\b`)
	eventHandlers   = regexp.MustCompile(`(?i)\son[a-z]+\s*=`)
	jsURLs          = regexp.MustCompile(`(?i)javascript\s*:`)
	styleLinks      = regexp.MustCompile(`(?i)<\s*link\b[^>]*stylesheet|@import\b`)
	imgSrc          = regexp.MustCompile(`(?i)<\s*img\b[^>]*\bsrc\s*=\s*['"]([^'"]+)['"]`)
	fieldRef        = regexp.MustCompile(`\{\{-?\s*\.([A-Za-z][A-Za-z0-9]*)\s*-?\}\}`)
)

// Validate checks an admin template before it is stored. warnings are
// non-blocking (https images). A non-nil error means nothing is saved.
func Validate(kind, subject, html, textBody string) (warnings []string, err error) {
	if !knownKind(kind) {
		return nil, fmt.Errorf("unknown email kind")
	}
	subject = strings.TrimSpace(subject)
	if subject == "" || strings.ContainsAny(subject, "\r\n") || len(subject) > 200 {
		return nil, fmt.Errorf("subject must be one non-empty line")
	}
	if !utf8.ValidString(subject) || !utf8.ValidString(html) || !utf8.ValidString(textBody) {
		return nil, fmt.Errorf("template must be valid UTF-8")
	}
	if len(html) == 0 || len(html) > MaxTemplateBytes {
		return nil, fmt.Errorf("HTML must be present and at most 200 KB")
	}
	if len(textBody) > MaxTemplateBytes {
		return nil, fmt.Errorf("plain text must be at most 200 KB")
	}
	if msg := rejectUnsafe(html); msg != "" {
		return nil, fmt.Errorf("%s", msg)
	}
	warnings = imageWarnings(html)
	if err := parseTree(html, kind, true); err != nil {
		return nil, err
	}
	textBody = strings.TrimSpace(textBody)
	if textBody != "" {
		if err := parseTree(textBody, kind, false); err != nil {
			return nil, err
		}
	}
	if err := requireFields(kind, html, textBody); err != nil {
		return nil, err
	}
	sample := SampleData(kind)
	if _, _, err := Execute(kind, html, textBody, sample); err != nil {
		return nil, fmt.Errorf("sample render: %w", err)
	}
	return warnings, nil
}

// Execute renders a stored template with the kind's whitelist. Amount
// and other fields are already formatted; no functions are available.
func Execute(kind, html, textBody string, data models.EmailData) (string, string, error) {
	vals := fieldMap(kind, data)
	h, err := execHTML(html, vals)
	if err != nil {
		return "", "", err
	}
	textBody = strings.TrimSpace(textBody)
	if textBody == "" {
		return h, HTMLToText(h), nil
	}
	t, err := execText(textBody, vals)
	if err != nil {
		return "", "", err
	}
	return h, t, nil
}

// HTMLToText is a small plain-text fallback when the admin leaves the
// text body empty. It is not a general HTML parser.
func HTMLToText(html string) string {
	s := regexp.MustCompile(`(?i)<\s*br\s*/?\s*>`).ReplaceAllString(html, "\n")
	s = regexp.MustCompile(`(?i)</\s*p\s*>`).ReplaceAllString(s, "\n\n")
	s = regexp.MustCompile(`(?i)<[^>]+>`).ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	lines := strings.Split(s, "\n")
	var kept []string
	blank := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			blank++
			if blank > 1 {
				continue
			}
			kept = append(kept, "")
			continue
		}
		blank = 0
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func knownKind(kind string) bool {
	for _, k := range models.EmailKinds {
		if k == kind {
			return true
		}
	}
	return false
}

func rejectUnsafe(html string) string {
	switch {
	case forbiddenBlocks.MatchString(html):
		return "template, define, and block actions are not allowed"
	case forbiddenTags.MatchString(html):
		return "script, iframe, form, object, and embed tags are not allowed"
	case eventHandlers.MatchString(html):
		return "event-handler attributes are not allowed"
	case jsURLs.MatchString(html):
		return "javascript URLs are not allowed"
	case styleLinks.MatchString(html):
		return "external stylesheets are not allowed; use inline CSS"
	}
	for _, src := range imgSrc.FindAllStringSubmatch(html, -1) {
		u := strings.TrimSpace(src[1])
		low := strings.ToLower(u)
		if strings.HasPrefix(low, "https://") {
			continue
		}
		return "images must use https URLs"
	}
	return ""
}

func imageWarnings(html string) []string {
	var out []string
	for _, src := range imgSrc.FindAllStringSubmatch(html, -1) {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(src[1])), "https://") {
			out = append(out, "External https image: "+src[1])
		}
	}
	return out
}

func requireFields(kind, html, textBody string) error {
	needAny := []string{}
	needAll := []string{}
	switch kind {
	case models.EmailPurchaseCourse, models.EmailPurchaseSoftware, models.EmailPurchaseArticle, models.EmailContribution:
		needAny = []string{"ReceiptURL", "AccessURL"}
	case models.EmailBookingNew, models.EmailBookingRescheduled, models.EmailBookingCancelled, models.EmailConnectRequest:
		needAll = []string{"ManageURL"}
	case models.EmailNewsletterConfirm:
		needAll = []string{"UnsubscribeURL"}
	}
	if len(needAny) > 0 && !hasAny(html, needAny) {
		return fmt.Errorf("include {{.ReceiptURL}} or {{.AccessURL}}")
	}
	for _, f := range needAll {
		if !hasField(html, f) {
			return fmt.Errorf("include {{.%s}}", f)
		}
		if strings.TrimSpace(textBody) != "" && !hasField(textBody, f) {
			return fmt.Errorf("plain text must include {{.%s}}", f)
		}
	}
	return nil
}

func hasAny(src string, fields []string) bool {
	for _, f := range fields {
		if hasField(src, f) {
			return true
		}
	}
	return false
}

func hasField(src, field string) bool {
	for _, m := range fieldRef.FindAllStringSubmatch(src, -1) {
		if m[1] == field {
			return true
		}
	}
	return false
}

func parseTree(src, kind string, htmlMode bool) error {
	if forbiddenBlocks.MatchString(src) {
		return fmt.Errorf("template, define, and block actions are not allowed")
	}
	var tree *parse.Tree
	if htmlMode {
		t, err := template.New("email").Option("missingkey=error").Parse(src)
		if err != nil {
			return fmt.Errorf("HTML template: %w", err)
		}
		tree = t.Tree
	} else {
		t, err := texttemplate.New("email").Option("missingkey=error").Parse(src)
		if err != nil {
			return fmt.Errorf("text template: %w", err)
		}
		tree = t.Tree
	}
	if err := rejectCalls(tree.Root); err != nil {
		return err
	}
	if _, err := execHTMLOrText(htmlMode, src, fieldMap(kind, SampleData(kind))); err != nil {
		return err
	}
	return nil
}

func execHTMLOrText(htmlMode bool, src string, vals map[string]any) (string, error) {
	if htmlMode {
		return execHTML(src, vals)
	}
	return execText(src, vals)
}

func execHTML(src string, vals map[string]any) (string, error) {
	t, err := template.New("email").Option("missingkey=error").Parse(src)
	if err != nil {
		return "", err
	}
	if err := rejectCalls(t.Tree.Root); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vals); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func execText(src string, vals map[string]any) (string, error) {
	t, err := texttemplate.New("email").Option("missingkey=error").Parse(src)
	if err != nil {
		return "", err
	}
	if err := rejectCalls(t.Tree.Root); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vals); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func rejectCalls(n parse.Node) error {
	if n == nil {
		return nil
	}
	switch node := n.(type) {
	case *parse.ListNode:
		for _, c := range node.Nodes {
			if err := rejectCalls(c); err != nil {
				return err
			}
		}
	case *parse.ActionNode:
		return rejectPipe(node.Pipe)
	case *parse.IfNode:
		if err := rejectPipe(node.Pipe); err != nil {
			return err
		}
		if err := rejectCalls(node.List); err != nil {
			return err
		}
		return rejectCalls(node.ElseList)
	case *parse.RangeNode:
		if err := rejectPipe(node.Pipe); err != nil {
			return err
		}
		if err := rejectCalls(node.List); err != nil {
			return err
		}
		return rejectCalls(node.ElseList)
	case *parse.WithNode:
		if err := rejectPipe(node.Pipe); err != nil {
			return err
		}
		if err := rejectCalls(node.List); err != nil {
			return err
		}
		return rejectCalls(node.ElseList)
	case *parse.TemplateNode:
		return fmt.Errorf("template actions are not allowed")
	}
	return nil
}

func rejectPipe(p *parse.PipeNode) error {
	if p == nil {
		return nil
	}
	for _, cmd := range p.Cmds {
		if len(cmd.Args) == 0 {
			continue
		}
		if _, ok := cmd.Args[0].(*parse.IdentifierNode); ok {
			return fmt.Errorf("function calls are not allowed")
		}
		for _, arg := range cmd.Args {
			if err := rejectCalls(arg); err != nil {
				return err
			}
		}
	}
	return nil
}

func fieldMap(kind string, data models.EmailData) map[string]any {
	all := map[string]any{
		"BuyerName":      data.BuyerName,
		"ItemTitle":      data.ItemTitle,
		"Amount":         data.Amount,
		"ReceiptNo":      data.ReceiptNo,
		"ReceiptURL":     data.ReceiptURL,
		"AccessURL":      data.AccessURL,
		"ClaimCode":      data.ClaimCode,
		"Instructions":   data.Instructions,
		"BookingStart":   data.BookingStart,
		"BookingCode":    data.BookingCode,
		"Duration":       data.Duration,
		"ManageURL":      data.ManageURL,
		"UnsubscribeURL": data.UnsubscribeURL,
		"SiteName":       data.SiteName,
		"SupportReplyTo": data.SupportReplyTo,
	}
	out := map[string]any{}
	for _, name := range models.EmailFields(kind) {
		out[name] = all[name]
	}
	return out
}
