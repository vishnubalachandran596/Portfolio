package notify

import (
	"strings"
	"testing"
)

func TestWithAdminBCCDedupes(t *testing.T) {
	buyer := WithAdminBCC(Message{To: "buyer@example.com"}, " Owner@Example.com ")
	if len(buyer.Bcc) != 1 || buyer.Bcc[0] != "owner@example.com" {
		t.Fatalf("buyer bcc: %#v", buyer.Bcc)
	}
	again := WithAdminBCC(Message{To: "buyer@example.com", Bcc: []string{"owner@example.com", " OWNER@example.com "}}, "owner@example.com")
	if len(again.Bcc) != 1 || again.Bcc[0] != "owner@example.com" {
		t.Fatalf("deduped bcc: %#v", again.Bcc)
	}
	self := WithAdminBCC(Message{To: "owner@example.com"}, "owner@example.com")
	if len(self.Bcc) != 0 {
		t.Fatalf("admin recipient was copied onto bcc: %#v", self.Bcc)
	}
	empty := WithAdminBCC(Message{To: "buyer@example.com"}, "  ")
	if len(empty.Bcc) != 0 {
		t.Fatalf("empty admin: %#v", empty.Bcc)
	}
}

func TestDisabledMailer(t *testing.T) {
	id, err := (Disabled{}).Send(t.Context(), Message{To: "a@b.c"})
	if err != ErrDisabled || id != "" {
		t.Fatalf("disabled: id %q err %v", id, err)
	}
	if (Disabled{}).Name() != "disabled" {
		t.Fatal((Disabled{}).Name())
	}
}

func TestFromFor(t *testing.T) {
	if !strings.Contains(FromFor("booking_new"), "bookings@") {
		t.Fatal(FromFor("booking_new"))
	}
	if !strings.Contains(FromFor("newsletter_confirm"), "news@") {
		t.Fatal(FromFor("newsletter_confirm"))
	}
	if !strings.Contains(FromFor("purchase_course"), "orders@") {
		t.Fatal(FromFor("purchase_course"))
	}
	if ReplyTo != "admin@vishnubalachandran.com" {
		t.Fatal(ReplyTo)
	}
}
