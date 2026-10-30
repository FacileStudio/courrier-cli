package cmd

import (
	"reflect"
	"strings"
	"testing"

	"github.com/FacileStudio/courrier-cli/internal/client"
)

func sampleMail() client.Email {
	return client.Email{
		ID: 7, MessageID: "<m2@x>", References: "<m1@x>", Subject: "Hello", Date: "2026-10-30",
		FromAddress: "bob@x.io", BodyText: "line one\nline two",
		ToAddresses: []client.Address{{Email: "me@x.io"}, {Email: "ann@x.io"}},
		CcAddresses: []client.Address{{Email: "BOB@x.io"}, {Email: "cy@x.io"}},
	}
}

func TestPrefixedAddsOnce(t *testing.T) {
	for in, want := range map[string]string{"Hi": "Re: Hi", "re: Hi": "re: Hi", " RE: Hi ": "RE: Hi"} {
		if got := prefixed("Re: ", in); got != want {
			t.Errorf("prefixed(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReplyRecipients(t *testing.T) {
	to, cc := replyRecipients(sampleMail(), false, "me@x.io")
	if !reflect.DeepEqual(to, []string{"bob@x.io"}) || cc != nil {
		t.Errorf("plain reply = %v %v", to, cc)
	}
	to, cc = replyRecipients(sampleMail(), true, "ME@x.io")
	if !reflect.DeepEqual(to, []string{"bob@x.io"}) || !reflect.DeepEqual(cc, []string{"ann@x.io", "cy@x.io"}) {
		t.Errorf("reply all = %v %v", to, cc)
	}
}

func TestThreadReferencesAndQuote(t *testing.T) {
	if got := threadReferences(sampleMail()); !reflect.DeepEqual(got, []string{"<m1@x>", "<m2@x>"}) {
		t.Errorf("references = %v", got)
	}
	quoted := quoteText(sampleMail())
	if !strings.Contains(quoted, "> line one\n> line two") || !strings.HasPrefix(quoted, "On 2026-10-30") {
		t.Errorf("quote = %q", quoted)
	}
}

func TestSafeNameDropsDirectories(t *testing.T) {
	for in, want := range map[string]string{"../../etc/passwd": "passwd", "a\\b.txt": "b.txt", "..": "attachment", "": "attachment"} {
		if got := safeName(in); got != want {
			t.Errorf("safeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildForwardKeepsSubjectAndAddsBlock(t *testing.T) {
	note := "fyi"
	req := buildForward(composeFlags{to: []string{"z@x.io"}}, sampleMail(), &note, nil)
	if req.Subject != "Fwd: Hello" || !strings.HasPrefix(req.Body, "fyi\n\n---------- Forwarded message") {
		t.Errorf("forward = %+v", req)
	}
}
