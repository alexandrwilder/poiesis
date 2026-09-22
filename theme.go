package main

import "github.com/charmbracelet/lipgloss"

// A theme is one decision: the colours of the text, lines and buttons, and how the camera
// picture is treated under them. Each is checked for contrast: the text sits on a darkened
// patch of the picture, so every text colour must stay bright enough to read there, and
// the frame lines must show over a bright picture as well as a dark one.

type theme struct {
	Name, Note                 string
	Accent, Accent2, Ink, Mid  string
	Dim, Rule, OK, End, Button string
	Look                       string  // the camera treatment, from look.go
	Strength                   float64 // how much of the look, 0..1
	Patch                      float64 // how dark the patch behind text is: lower is darker (0.2..0.5)
}

var themes = []theme{
	// professional
	{Name: "ember", Note: "warm amber, the picture faded · the log's own look", Accent: "#F0B052", Accent2: "#F7D28C", Ink: "#EFECE4", Mid: "#B9BCC2", Dim: "#8A8F98", Rule: "#6A6F78", OK: "#8ED39B", End: "#F07A5C", Button: "#3A3E46", Look: "faded", Strength: 0.7, Patch: 0.35},
	{Name: "frost", Note: "ice white on a cool picture", Accent: "#DCEBFF", Accent2: "#A9D2FF", Ink: "#F3F7FC", Mid: "#C3D1E2", Dim: "#8FA0B6", Rule: "#728499", OK: "#A8E6D4", End: "#FF9C8A", Button: "#324255", Look: "cool", Strength: 0.6, Patch: 0.35},
	{Name: "paper", Note: "warm white on sepia, like an old print", Accent: "#FFE1A8", Accent2: "#FFF1D6", Ink: "#FAF3E6", Mid: "#D9CDB8", Dim: "#A99C86", Rule: "#8A7E6A", OK: "#B7E0A8", End: "#FF9A7A", Button: "#4A4033", Look: "sepia", Strength: 0.8, Patch: 0.35},
	{Name: "mono", Note: "white on black and white", Accent: "#FFFFFF", Accent2: "#E6E6E6", Ink: "#F4F4F4", Mid: "#C4C4C4", Dim: "#9A9A9A", Rule: "#7C7C7C", OK: "#A6E7B5", End: "#FF8B7A", Button: "#3A3A3A", Look: "mono", Strength: 0.9, Patch: 0.3},
	// cinematic
	{Name: "noir", Note: "hard black and white, bright text", Accent: "#FFFFFF", Accent2: "#FFD45C", Ink: "#FFFFFF", Mid: "#D0D0D0", Dim: "#A0A0A0", Rule: "#8A8A8A", OK: "#B9F0C4", End: "#FF7A66", Button: "#404040", Look: "noir", Strength: 1, Patch: 0.25},
	{Name: "deep", Note: "ice blue over a night picture", Accent: "#CFE6FF", Accent2: "#8FD0FF", Ink: "#EAF3FF", Mid: "#B4C8E0", Dim: "#8AA0BC", Rule: "#7189A6", OK: "#9FE8D6", End: "#FF9A88", Button: "#2E3F55", Look: "night", Strength: 0.9, Patch: 0.4},
	{Name: "signal", Note: "amber and red on a warm picture · a mission log", Accent: "#FFB347", Accent2: "#FF7F5C", Ink: "#F5EFE6", Mid: "#C9BFB2", Dim: "#9C9184", Rule: "#7A7066", OK: "#9EDB9A", End: "#FF5C4A", Button: "#4A3A30", Look: "warm", Strength: 0.7, Patch: 0.35},
	{Name: "phosphor", Note: "green screen", Accent: "#9CFFB0", Accent2: "#DCFFB8", Ink: "#DDFBE3", Mid: "#9FD9AC", Dim: "#79B58A", Rule: "#5E9A6E", OK: "#9CFFB0", End: "#FF9A6A", Button: "#244A32", Look: "mono", Strength: 0.9, Patch: 0.3},
	// fun, for the lab
	{Name: "prism", Note: "many colours, the picture as it is", Accent: "#FFC15E", Accent2: "#7FD6FF", Ink: "#F7F4EE", Mid: "#D2C4F0", Dim: "#A59FB5", Rule: "#8B84A0", OK: "#8DF0B0", End: "#FF7B7B", Button: "#3E3660", Look: "true", Strength: 0, Patch: 0.35},
	{Name: "neon", Note: "cyan and magenta on a vivid picture", Accent: "#5CF5FF", Accent2: "#FF7BE9", Ink: "#F4FDFF", Mid: "#BFEFF5", Dim: "#8DBCC4", Rule: "#6FA6AE", OK: "#8DFFB8", End: "#FF6E8A", Button: "#233F4A", Look: "vivid", Strength: 0.8, Patch: 0.3},
	{Name: "lab", Note: "acid yellow on a clear picture", Accent: "#E9FF6A", Accent2: "#FFFFFF", Ink: "#F8FBEA", Mid: "#CCD6A8", Dim: "#9DA982", Rule: "#7E8A66", OK: "#B4FF8A", End: "#FF8C6A", Button: "#3E4630", Look: "true", Strength: 0, Patch: 0.35},
}

var current = themes[0]

func themeNames() []string {
	var out []string
	for _, t := range themes {
		out = append(out, t.Name)
	}
	return out
}

func themeByName(name string) theme {
	for _, t := range themes {
		if t.Name == name {
			return t
		}
	}
	return themes[0]
}

func hexRGB(h string) [3]uint8 {
	var v [3]uint8
	if len(h) == 7 {
		for i := 0; i < 3; i++ {
			v[i] = hexByte(h[1+2*i])<<4 | hexByte(h[2+2*i])
		}
	}
	return v
}

func hexByte(c byte) uint8 {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

// luminance of a hex colour, 0..1, the way the eye weighs it
func luminance(h string) float64 {
	c := hexRGB(h)
	return (0.299*float64(c[0]) + 0.587*float64(c[1]) + 0.114*float64(c[2])) / 255
}

// applyTheme recolours every style the screens use and sets the camera look.
func applyTheme(name string) {
	t := themeByName(name)
	current = t
	cAmber, cInk, cMid, cDim, cOK, cRule = lipgloss.Color(t.Accent), lipgloss.Color(t.Ink), lipgloss.Color(t.Mid), lipgloss.Color(t.Dim), lipgloss.Color(t.OK), lipgloss.Color(t.Rule)
	cAccent2 := lipgloss.Color(t.Accent2)
	sInk = lipgloss.NewStyle().Foreground(cInk)
	sMid = lipgloss.NewStyle().Foreground(cMid)
	sDim = lipgloss.NewStyle().Foreground(cDim)
	sAmber = lipgloss.NewStyle().Foreground(cAmber)
	sAmberB = lipgloss.NewStyle().Foreground(cAmber).Bold(true)
	sAccent2 = lipgloss.NewStyle().Foreground(cAccent2)
	sOK = lipgloss.NewStyle().Foreground(cOK)
	sHead = lipgloss.NewStyle().Foreground(cInk).Bold(true)
	sCursor = lipgloss.NewStyle().Foreground(cAmber)
	sRule = lipgloss.NewStyle().Foreground(cRule)
	sKey = lipgloss.NewStyle().Foreground(cInk).Background(lipgloss.Color(t.Button))
	sChip = lipgloss.NewStyle().Foreground(cMid).Padding(0, 1)
	sChipOn = lipgloss.NewStyle().Foreground(lipgloss.Color("#141210")).Background(cAmber).Bold(true).Padding(0, 1)
	sBar = lipgloss.NewStyle().Foreground(cInk)
	rgbInk, rgbMid, rgbDim, rgbAmber, rgbOK, rgbEnd = hexRGB(t.Ink), hexRGB(t.Mid), hexRGB(t.Dim), hexRGB(t.Accent), hexRGB(t.OK), hexRGB(t.End)
	rgbAccent2 = hexRGB(t.Accent2)
	rgbRule = hexRGB(t.Rule)
	currentLook = lookByName(t.Look)
	currentStrength = t.Strength
	textPatch = t.Patch
}

// textPatch is how much of the picture shows behind text: the theme decides.
var textPatch = 0.35
