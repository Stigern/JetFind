package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Palette from the approved "dark cards" design.
var (
	colBackground = color.NRGBA{0x14, 0x16, 0x1c, 0xff}
	colCard       = color.NRGBA{0x1e, 0x21, 0x29, 0xff}
	colBorder     = color.NRGBA{0x2a, 0x2e, 0x38, 0xff}
	colAccent     = color.NRGBA{0x2a, 0x6d, 0xf4, 0xff}
	colAccent2    = color.NRGBA{0x7b, 0x4d, 0xff, 0xff}
	colAccentText = color.NRGBA{0x8f, 0xb4, 0xff, 0xff}
	colText       = color.NRGBA{0xe8, 0xea, 0xf0, 0xff}
	colMuted      = color.NRGBA{0xa3, 0xa9, 0xba, 0xff}
	colFaint      = color.NRGBA{0x5d, 0x63, 0x72, 0xff}
	colOnline     = color.NRGBA{0x3d, 0xdc, 0x84, 0xff}
	colWarn       = color.NRGBA{0xf5, 0xb7, 0x3b, 0xff}
	colOffline    = color.NRGBA{0xef, 0x5d, 0x5d, 0xff}
)

// darkTheme forces the JetFind palette regardless of the OS light/dark setting.
type darkTheme struct{}

func (darkTheme) Color(n fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameBackground:
		return colBackground
	case theme.ColorNameForeground:
		return colText
	case theme.ColorNamePrimary, theme.ColorNameFocus:
		return colAccent
	case theme.ColorNameButton, theme.ColorNameInputBackground:
		return colCard
	case theme.ColorNameDisabledButton:
		return colBorder
	case theme.ColorNameDisabled, theme.ColorNamePlaceHolder:
		return colMuted
	case theme.ColorNameScrollBar:
		return colBorder
	case theme.ColorNameSeparator:
		return colBorder
	}
	return theme.DefaultTheme().Color(n, theme.VariantDark)
}

func (darkTheme) Font(s fyne.TextStyle) fyne.Resource     { return theme.DefaultTheme().Font(s) }
func (darkTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }

func (darkTheme) Size(n fyne.ThemeSizeName) float32 {
	if n == theme.SizeNameInputRadius || n == theme.SizeNameSelectionRadius {
		return 8
	}
	return theme.DefaultTheme().Size(n)
}
