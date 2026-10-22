package macapplication

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"
)

// plistNode is one element of a parsed XML plist tree. Leaf elements carry
// their (normalized) text; container elements carry children.
type plistNode struct {
	name     string
	attrs    []xml.Attr
	children []*plistNode
	text     string
}

// canonicalPlistSHA256 returns the hex SHA-256 of canonicalPlist(b). Two
// plists that differ only in formatting (insignificant whitespace, <dict>
// key order, XML declaration/DOCTYPE, comments) hash identically.
func canonicalPlistSHA256(b []byte) (string, error) {
	c, err := canonicalPlist(b)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(c)
	return hex.EncodeToString(sum[:]), nil
}

// canonicalPlist parses an XML property list and re-emits it in a
// deterministic canonical form:
//   - the XML declaration, DOCTYPE, comments and processing instructions are
//     dropped;
//   - whitespace-only text between elements is dropped;
//   - <dict> entries are sorted by <key> (array order is preserved);
//   - leaf text is normalized: <string>/<key> text is kept exactly (its
//     whitespace is data); non-string scalars (integer, real, date,
//     true/false) are trimmed; for <data> all whitespace is removed from the
//     base64;
//   - attributes are emitted sorted by name.
//
// Non-XML input — including a binary plist ("bplist00") — is an error; the
// caller must then treat the plist as not-equal to anything.
func canonicalPlist(b []byte) ([]byte, error) {
	if bytes.HasPrefix(bytes.TrimSpace(b), []byte("bplist")) {
		return nil, errors.New("binary plist is not supported for canonicalization")
	}

	root, err := parsePlistTree(b)
	if err != nil {
		return nil, err
	}
	if err := normalizePlistNode(root); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	emitPlistNode(&buf, root)
	return buf.Bytes(), nil
}

func parsePlistTree(b []byte) (*plistNode, error) {
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.Strict = true

	var root *plistNode
	var stack []*plistNode
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("plist is not well-formed XML: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &plistNode{name: t.Name.Local}
			for _, a := range t.Attr {
				n.attrs = append(n.attrs, xml.Attr{Name: xml.Name{Local: a.Name.Local}, Value: a.Value})
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, errors.New("plist has more than one root element")
				}
				root = n
			} else {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 0 {
				if strings.TrimSpace(string(t)) != "" {
					return nil, errors.New("plist has text outside the root element")
				}
				continue
			}
			stack[len(stack)-1].text += string(t)
		default:
			// xml.ProcInst (declaration), xml.Directive (DOCTYPE) and
			// xml.Comment carry no plist data.
		}
	}
	if root == nil {
		return nil, errors.New("input has no XML root element")
	}
	return root, nil
}

func normalizePlistNode(n *plistNode) error {
	if len(n.children) > 0 {
		if strings.TrimSpace(n.text) != "" {
			return fmt.Errorf("plist element <%s> mixes text and child elements", n.name)
		}
		n.text = ""
		for _, c := range n.children {
			if err := normalizePlistNode(c); err != nil {
				return err
			}
		}
		if n.name == "dict" {
			return sortPlistDict(n)
		}
		return nil
	}

	switch n.name {
	case "string", "key":
		// Leading/trailing whitespace in <string>/<key> is data: keep the
		// (entity/CDATA-decoded) text exactly.
	case "data":
		n.text = strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, n.text)
	default:
		// Non-string scalars (integer, real, date, true, false) and empty
		// containers (<dict>, <array>, <plist> holding only indentation):
		// surrounding whitespace is not significant.
		n.text = strings.TrimSpace(n.text)
	}
	return nil
}

// sortPlistDict reorders a <dict>'s (key, value) pairs by key text.
func sortPlistDict(n *plistNode) error {
	if len(n.children)%2 != 0 {
		return errors.New("plist <dict> has an unpaired key or value")
	}
	type entry struct {
		key   *plistNode
		value *plistNode
	}
	entries := make([]entry, 0, len(n.children)/2)
	for i := 0; i < len(n.children); i += 2 {
		if n.children[i].name != "key" {
			return fmt.Errorf("plist <dict> entry %d starts with <%s>, want <key>", i/2, n.children[i].name)
		}
		entries = append(entries, entry{key: n.children[i], value: n.children[i+1]})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].key.text < entries[j].key.text })
	n.children = n.children[:0]
	for _, e := range entries {
		n.children = append(n.children, e.key, e.value)
	}
	return nil
}

func emitPlistNode(buf *bytes.Buffer, n *plistNode) {
	buf.WriteByte('<')
	buf.WriteString(n.name)
	attrs := append([]xml.Attr(nil), n.attrs...)
	sort.Slice(attrs, func(i, j int) bool { return attrs[i].Name.Local < attrs[j].Name.Local })
	for _, a := range attrs {
		buf.WriteByte(' ')
		buf.WriteString(a.Name.Local)
		buf.WriteString(`="`)
		_ = xml.EscapeText(buf, []byte(a.Value))
		buf.WriteByte('"')
	}
	buf.WriteByte('>')
	if len(n.children) > 0 {
		for _, c := range n.children {
			emitPlistNode(buf, c)
		}
	} else {
		_ = xml.EscapeText(buf, []byte(n.text))
	}
	buf.WriteString("</")
	buf.WriteString(n.name)
	buf.WriteByte('>')
}
