package handler

// AskRequest is one link that needs a choice of target.
type AskRequest struct {
	// ID identifies the link (opener and URL): a second request with the same ID while the first panel is open
	// joins it.
	ID string
	// Title is what the panel shows about the link (host and path); it is shown as plain text.
	Title string
	// Options are the button labels, 1 to 9, in order.
	Options []string
	// CanRemember enables the "Remember" checkbox.
	CanRemember bool
	// RememberLabel is the text of the checkbox, shown as plain text; empty keeps the default text.
	RememberLabel string
}

// AskResult is the user's answer.
type AskResult struct {
	// Choice is the index in Options, or -1 when the panel was cancelled (Esc, closed, the app is quitting).
	Choice   int
	Remember bool
	// Joined: an identical link is already waiting in a panel; this one must not be opened.
	Joined bool
}

// Cancelled is the answer that opens the default target.
var Cancelled = AskResult{Choice: -1}
