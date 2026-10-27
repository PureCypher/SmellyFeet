package server

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// sanitizeArticleHTML renders untrusted publisher HTML through a strict
// whitelist so a stored article body can be displayed with its original
// structure (headings, paragraphs, lists, images) without turning the page
// into an XSS vector. Treat everything in `raw` as hostile:
//
//   - The input is fully parsed (never re-serialized token-for-token), so
//     malformed or truncated markup is normalized before any of it is emitted.
//   - Only whitelisted tags survive; everything else either has its children
//     kept (unwrap: div/section/span chrome) or its subtree dropped
//     (script/iframe/svg widgets).
//   - All attributes are dropped except a per-tag allowlist, and each allowed
//     value is re-validated (scheme checks, numeric-only width/height).
//   - Image sources are rewritten to the same-origin /img proxy — the page's
//     CSP stays img-src 'self', and the proxy enforces its own SSRF guards.
//   - Text nodes are re-escaped on output; the caller embeds the result as
//     template.HTML, so this function is the only escape hatch in the app and
//     must never be bypassed.
//
// pageURL is the article's publisher URL, used to resolve relative hrefs and
// image srcs to absolute ones. Unresolvable URLs are dropped, not guessed.
func sanitizeArticleHTML(raw, pageURL string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	// Full DOM parse: normalizes malformed/truncated markup into a tree.
	parsed, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return ""
	}
	var out strings.Builder
	walkSanitized(&out, findNode(parsed, html.ElementNode, "body"), sanitizeCtx{
		pageURL: pageURL,
	})
	return out.String()
}

type sanitizeCtx struct {
	pageURL string
}

// nodeAction decides what happens to an element during sanitization.
type nodeAction int

const (
	actionKeep   nodeAction = iota // render the tag (with whitelisted attrs)
	actionUnwrap                   // drop the tag, keep rendering its children
	actionDrop                     // drop the tag and its whole subtree
)

// sanitizeRules maps every element the parser can produce to an action, plus
// the attributes allowed on kept tags. Anything absent from this map defaults
// to unwrap — unknown tags never become markup.
var sanitizeRules = map[string]struct {
	action nodeAction
	attrs  []string
}{
	"a":          {actionKeep, []string{"href", "title"}},
	"abbr":       {actionKeep, []string{"title"}},
	"b":          {actionKeep, nil},
	"blockquote": {actionKeep, []string{"cite"}},
	"br":         {actionKeep, nil},
	"cite":       {actionKeep, nil},
	"code":       {actionKeep, nil},
	"dd":         {actionKeep, nil},
	"del":        {actionKeep, nil},
	"dl":         {actionKeep, nil},
	"dt":         {actionKeep, nil},
	"em":         {actionKeep, nil},
	"figcaption": {actionKeep, nil},
	"figure":     {actionKeep, nil},
	"h1":         {actionKeep, nil},
	"h2":         {actionKeep, nil},
	"h3":         {actionKeep, nil},
	"h4":         {actionKeep, nil},
	"h5":         {actionKeep, nil},
	"h6":         {actionKeep, nil},
	"hr":         {actionKeep, nil},
	"i":          {actionKeep, nil},
	"img":        {actionKeep, []string{"src", "alt", "width", "height", "title"}},
	"ins":        {actionKeep, nil},
	"kbd":        {actionKeep, nil},
	"li":         {actionKeep, nil},
	"mark":       {actionKeep, nil},
	"ol":         {actionKeep, []string{"start"}},
	"p":          {actionKeep, nil},
	"pre":        {actionKeep, nil},
	"q":          {actionKeep, []string{"cite"}},
	"s":          {actionKeep, nil},
	"samp":       {actionKeep, nil},
	"small":      {actionKeep, nil},
	"strong":     {actionKeep, nil},
	"sub":        {actionKeep, nil},
	"sup":        {actionKeep, nil},
	"table":      {actionKeep, nil},
	"tbody":      {actionKeep, nil},
	"td":         {actionKeep, []string{"colspan", "rowspan"}},
	"tfoot":      {actionKeep, nil},
	"th":         {actionKeep, []string{"colspan", "rowspan", "scope"}},
	"thead":      {actionKeep, nil},
	"tr":         {actionKeep, nil},
	"u":          {actionKeep, nil},
	"ul":         {actionKeep, nil},

	// Structural chrome: keep the text/images inside, lose the tag.
	"div":      {actionUnwrap, nil},
	"section":  {actionUnwrap, nil},
	"article":  {actionUnwrap, nil},
	"main":     {actionUnwrap, nil},
	"span":     {actionUnwrap, nil},
	"font":     {actionUnwrap, nil},
	"center":   {actionUnwrap, nil},
	"time":     {actionUnwrap, nil},
	"details":  {actionUnwrap, nil},
	"summary":  {actionUnwrap, nil},
	"caption":  {actionUnwrap, nil},
	"colgroup": {actionUnwrap, nil},
	"picture":  {actionUnwrap, nil},
	"source":   {actionUnwrap, nil},
	"hgroup":   {actionUnwrap, nil},

	// Widgets and metadata: drop the entire subtree.
	"script":   {actionDrop, nil},
	"style":    {actionDrop, nil},
	"noscript": {actionDrop, nil},
	"template": {actionDrop, nil},
	"iframe":   {actionDrop, nil},
	"frame":    {actionDrop, nil},
	"frameset": {actionDrop, nil},
	"object":   {actionDrop, nil},
	"embed":    {actionDrop, nil},
	"applet":   {actionDrop, nil},
	"form":     {actionDrop, nil},
	"input":    {actionDrop, nil},
	"select":   {actionDrop, nil},
	"option":   {actionDrop, nil},
	"textarea": {actionDrop, nil},
	"button":   {actionDrop, nil},
	"video":    {actionDrop, nil},
	"audio":    {actionDrop, nil},
	"canvas":   {actionDrop, nil},
	"svg":      {actionDrop, nil},
	"math":     {actionDrop, nil},
	"nav":      {actionDrop, nil},
	"aside":    {actionDrop, nil},
	"header":   {actionDrop, nil},
	"footer":   {actionDrop, nil},
	"head":     {actionDrop, nil},
	"title":    {actionDrop, nil},
	"meta":     {actionDrop, nil},
	"link":     {actionDrop, nil},
	"base":     {actionDrop, nil},
	"dialog":   {actionDrop, nil},
	"col":      {actionDrop, nil},
}

// voidElements never get a closing tag.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

// findNode returns the first descendant of n matching type+tag.
func findNode(n *html.Node, typ html.NodeType, tag string) *html.Node {
	if n == nil {
		return nil
	}
	if n.Type == typ && n.Data == tag {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findNode(c, typ, tag); found != nil {
			return found
		}
	}
	return nil
}

// walkSanitized renders the subtree rooted at n into out, applying the rules
// above. Pre-order traversal; text nodes are re-escaped, comment/doctype
// nodes are dropped.
func walkSanitized(out *strings.Builder, n *html.Node, ctx sanitizeCtx) {
	for c := n; c != nil; c = c.NextSibling {
		switch c.Type {
		case html.ElementNode:
			rule, ok := sanitizeRules[c.Data]
			switch {
			case ok && rule.action == actionKeep:
				if c.Data == "img" && !imgHasValidSrc(c, ctx) {
					// src failed scheme/URL validation — render nothing
					// rather than a broken-image placeholder.
					continue
				}
				// The page's own <h1> is the article title; a publisher h1
				// inside the body would break heading hierarchy, so it
				// demotes to <h2>. (Extraction strips byline headers, but a
				// stray h1 can still arrive outside a <header>.)
				tag := c.Data
				if tag == "h1" {
					tag = "h2"
				}
				out.WriteString("<")
				out.WriteString(tag)
				writeSanitizedAttrs(out, c, ctx)
				out.WriteString(">")
				if !voidElements[c.Data] {
					walkSanitized(out, c.FirstChild, ctx)
					out.WriteString("</" + tag + ">")
				}
			case !ok || rule.action == actionUnwrap:
				// Unknown tag (including the body root we start at) or
				// explicit unwrap: drop the tag, keep rendering children.
				// Only explicit actionDrop reaches no case: emit nothing.
				walkSanitized(out, c.FirstChild, ctx)
			}
		case html.TextNode:
			out.WriteString(html.EscapeString(c.Data))
		}
		// Comments and doctypes are dropped.
	}
}

// imgHasValidSrc reports whether the img's src survives validation (i.e. it
// rewrites to a proxied same-origin URL). An img without a usable src is
// dropped entirely.
func imgHasValidSrc(n *html.Node, ctx sanitizeCtx) bool {
	for _, a := range n.Attr {
		if a.Key == "src" {
			return allowedAttrValue("img", "src", a.Val, ctx) != ""
		}
	}
	return false
}

// writeSanitizedAttrs emits only the attributes whitelisted for the element,
// with values re-validated. It always runs AFTER parsing, so nothing here
// can smuggle new markup — values are escaped individually.
func writeSanitizedAttrs(out *strings.Builder, n *html.Node, ctx sanitizeCtx) {
	for _, a := range n.Attr {
		val := allowedAttrValue(n.Data, a.Key, a.Val, ctx)
		if val == "" {
			continue
		}
		out.WriteString(" ")
		out.WriteString(a.Key)
		out.WriteString(`="`)
		out.WriteString(html.EscapeString(val))
		out.WriteString(`"`)
	}
	if n.Data == "img" {
		// Lazy-load + same-origin fetch for body images.
		out.WriteString(` loading="lazy" decoding="async"`)
	}
	if n.Data == "a" {
		// Publisher links open in a new tab and never pass this site's
		// referrer or link equity.
		out.WriteString(` target="_blank" rel="noopener noreferrer nofollow"`)
	}
}

// allowedAttrValue validates one attribute for one tag. Empty string means
// "drop the attribute".
func allowedAttrValue(tag, key, val string, ctx sanitizeCtx) string {
	val = strings.TrimSpace(val)
	switch {
	case tag == "a" && key == "href":
		return sanitizeLink(val, ctx.pageURL)
	case tag == "blockquote" && key == "cite", tag == "q" && key == "cite":
		if u := absoluteURL(val, ctx.pageURL); u != nil && isHTTPScheme(u.Scheme) {
			return u.String()
		}
		return ""
	case tag == "img" && key == "src":
		// Rewrite to the same-origin image proxy (keeps CSP img-src 'self').
		u := absoluteURL(val, ctx.pageURL)
		if u == nil || !isHTTPScheme(u.Scheme) {
			return "" // data:/javascript:/relative-unresolvable: drop the img
		}
		return "/img?u=" + url.QueryEscape(u.String())
	case tag == "img" && (key == "width" || key == "height") ||
		tag == "td" && (key == "colspan" || key == "rowspan") ||
		tag == "th" && (key == "colspan" || key == "rowspan") ||
		tag == "ol" && key == "start":
		if isDigits(val) {
			return val
		}
		return ""
	case tag == "img" && key == "alt":
		// Alt text is essential accessibility data; escape with the markup.
		return val
	default:
		// title (a/abbr/img) and scope (th): plain text values.
		if key == "title" || key == "scope" {
			return val
		}
		return ""
	}
}

// sanitizeLink resolves href against the article URL and keeps only http,
// https, and mailto links. Anything else (javascript:, data:, vbscript:,
// unknown schemes, unresolvable relatives) loses the href and renders as a
// non-link anchor.
func sanitizeLink(href, pageURL string) string {
	if href == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(href), "mailto:") {
		return href
	}
	u := absoluteURL(href, pageURL)
	if u == nil || !isHTTPScheme(u.Scheme) {
		return ""
	}
	return u.String()
}

// absoluteURL resolves ref against the publisher's URL. Returns nil when
// either URL is unusable — never fall back to the raw string.
func absoluteURL(ref, pageURL string) *url.URL {
	base, err := url.Parse(pageURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil
	}
	u, err := base.Parse(ref)
	if err != nil {
		return nil
	}
	return u
}

func isHTTPScheme(s string) bool {
	return s == "http" || s == "https"
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
