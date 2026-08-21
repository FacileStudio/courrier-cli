package client

// AuthConfig is what an instance says it accepts before a login is attempted.
// SSOOnly means the local password routes are not registered at all, so a
// password login answers 404 rather than 401.
type AuthConfig struct {
	SSOOnly     bool `json:"sso_only"`
	OIDCEnabled bool `json:"oidc_enabled"`
}

// Account is one configured IMAP/SMTP mailbox. Passwords are never returned.
type Account struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	IMAPHost  string `json:"imap_host"`
	IMAPPort  int    `json:"imap_port"`
	IMAPUser  string `json:"imap_user"`
	SMTPHost  string `json:"smtp_host"`
	SMTPPort  int    `json:"smtp_port"`
	SMTPUser  string `json:"smtp_user"`
	Signature string `json:"signature"`
	IsDefault bool   `json:"is_default"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Folder is one IMAP mailbox as the server knows it. Type is the normalised
// kind — inbox, sent, drafts, trash, junk, archive or custom — and is what the
// listing endpoint addresses folders by, not ID.
type Folder struct {
	ID          int64  `json:"id"`
	AccountID   int64  `json:"account_id"`
	Path        string `json:"path"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	UnreadCount int    `json:"unread_count"`
	TotalCount  int    `json:"total_count"`
}

// Address is one parsed mail address. Name is empty when the header carried
// only an address.
type Address struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Attachment is one file carried by an email. The bytes are not included; they
// are fetched separately by ID.
type Attachment struct {
	ID       int64  `json:"id"`
	Filename string `json:"filename"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
}

// Email is one message, or one collapsed conversation.
//
// MessageCount, UnreadCount and EmailIDs are present only on the collapsed
// conversation rows returned by ListEmails, where the row stands for a whole
// thread and carries the newest message's fields. Every other endpoint returns
// a single message and leaves those three at their zero values. EmailIDs is
// itself omitted for a conversation of one.
//
// BodyText and BodyHTML are populated when a single message is fetched; list
// and search rows carry neither.
type Email struct {
	ID             int64        `json:"id"`
	AccountID      int64        `json:"account_id"`
	FolderID       int64        `json:"folder_id"`
	MessageID      string       `json:"message_id"`
	ThreadID       string       `json:"thread_id,omitempty"`
	Subject        string       `json:"subject"`
	FromAddress    string       `json:"from_address"`
	FromName       string       `json:"from_name"`
	ToAddresses    []Address    `json:"to_addresses"`
	CcAddresses    []Address    `json:"cc_addresses"`
	Date           string       `json:"date"`
	BodyText       string       `json:"body_text,omitempty"`
	BodyHTML       string       `json:"body_html,omitempty"`
	IsRead         bool         `json:"is_read"`
	IsStarred      bool         `json:"is_starred"`
	HasAttachments bool         `json:"has_attachments"`
	Attachments    []Attachment `json:"attachments,omitempty"`
	InReplyTo      string       `json:"in_reply_to,omitempty"`
	References     string       `json:"references,omitempty"`
	MessageCount   int          `json:"message_count,omitempty"`
	UnreadCount    int          `json:"unread_count,omitempty"`
	EmailIDs       []int64      `json:"email_ids,omitempty"`
}

// EmailPage is one page of a folder listing or a search. Total counts every
// matching row, not the ones in this page.
type EmailPage struct {
	Emails []Email `json:"emails"`
	Total  int     `json:"total"`
	Page   int     `json:"page"`
	Limit  int     `json:"limit"`
}

// ListOptions is the paging shared by ListEmails and Search. Zero values are
// left off the query so the server applies its own defaults: a folder listing
// defaults to 50 per page, a search to 30, and both cap at 100. UnreadOnly is
// read by ListEmails and ignored by Search.
type ListOptions struct {
	Page       int
	Limit      int
	UnreadOnly bool
}

// SendRequest is one outgoing message. Attachments are local file paths, read
// at send time; the server parses at most 25 MB of multipart body and answers
// 413 above that.
type SendRequest struct {
	To          []string
	Cc          []string
	Subject     string
	Body        string
	BodyHTML    string
	InReplyTo   string
	References  []string
	Attachments []string
}
