package input

// Content represents processed input content.
type Content struct {
	// Type is the detected type of the input.
	Type Type

	// Raw is the original input value.
	Raw string

	// Data is the processed data (file contents, URL response, etc).
	Data []byte

	// Metadata contains additional information about the content.
	Metadata map[string]any
}

// String returns the data as a string.
func (c *Content) String() string {
	return string(c.Data)
}

// NewContent creates a new Content with the given type and raw value.
func NewContent(typ Type, raw string) *Content {
	return &Content{
		Type:     typ,
		Raw:      raw,
		Metadata: make(map[string]any),
	}
}
