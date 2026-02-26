package uiassets

import _ "embed"

//go:embed logo.png
var logoPNG []byte

//go:embed hub.css
var hubCSS string

//go:embed hub.js
var hubJS string

//go:embed hub.html
var hubHTML string

//go:embed note.css
var noteCSS string

func LogoPNG() []byte { return append([]byte(nil), logoPNG...) }
func HubCSS() string  { return hubCSS }
func HubJS() string   { return hubJS }
func HubHTML() string { return hubHTML }
func NoteCSS() string { return noteCSS }
