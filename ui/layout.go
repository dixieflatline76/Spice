package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

// Alignment specifies the horizontal alignment.
type Alignment int

const (
	alignLeft Alignment = iota
	alignCenter
	alignRight
	alignOpposed
)

// SplitAlign is a namespace for the Alignment constants.
var SplitAlign = struct {
	Left    Alignment // AlignLeft left-align both widgets.
	Center  Alignment // AlignCenter center-align both widgets.
	Right   Alignment // AlignRight right-align both widgets.
	Opposed Alignment // AlignOpposed align the first widget to the left and the second widget to the right.
}{
	Left:    alignLeft,
	Center:  alignCenter,
	Right:   alignRight,
	Opposed: alignOpposed,
}

// FirstWidgetProportion represents the predefined split ratios.
type FirstWidgetProportion int

const (
	oneThird     FirstWidgetProportion = iota // 1/3 - 2/3
	oneFourth                                 // 1/4 - 3/4
	oneFifth                                  // 1/5 - 4/5
	twoFifths                                 // 2/5 - 3/5
	twoThirds                                 // 2/3 - 1/3
	threeFourths                              // 3/4 - 1/4
	fourFifths                                // 4/5 - 1/5
	threeFifths                               // 3/5 - 2/5
)

// SplitProportion is a namespace for the FirstWidgetProportion constants.  This is the key change.
var SplitProportion = struct {
	OneThird     FirstWidgetProportion // OneThird 1/3 - 2/3
	OneFourth    FirstWidgetProportion // OneFourth 1/4 - 3/4
	OneFifth     FirstWidgetProportion // OneFifth 1/5 - 4/5
	TwoFifths    FirstWidgetProportion // TwoFifths 2/5 - 3/5
	TwoThirds    FirstWidgetProportion // TwoThirds 2/3 - 1/3
	ThreeFourths FirstWidgetProportion // ThreeFourths 3/4 - 1/4
	FourFifths   FirstWidgetProportion // FourFifths 4/5 - 1/5
	ThreeFifths  FirstWidgetProportion // ThreeFifths 3/5 - 2/5
}{
	OneThird:     oneThird,
	OneFourth:    oneFourth,
	OneFifth:     oneFifth,
	TwoFifths:    twoFifths,
	TwoThirds:    twoThirds,
	ThreeFourths: threeFourths,
	FourFifths:   fourFifths,
	ThreeFifths:  threeFifths,
}

// splitLayout is our custom layout.
type splitLayout struct {
	widget1    fyne.CanvasObject
	widget2    fyne.CanvasObject
	proportion FirstWidgetProportion
	alignment  Alignment
}

// MinSize calculates the minimum size.
func (s *splitLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	w1Size := s.widget1.MinSize()
	w2Size := s.widget2.MinSize()
	maxWidth := w1Size.Width + w2Size.Width
	maxHeight := fyne.Max(w1Size.Height, w2Size.Height)
	return fyne.NewSize(maxWidth, maxHeight)
}

// Layout arranges the widgets.
func (s *splitLayout) Layout(objects []fyne.CanvasObject, containerSize fyne.Size) {
	var widget1Width float32

	// Determine widget widths based on the FirstWidgetProportion.
	switch s.proportion {
	case oneThird: // Use the internal constants
		widget1Width = containerSize.Width / 3
	case oneFourth:
		widget1Width = containerSize.Width / 4
	case oneFifth:
		widget1Width = containerSize.Width / 5
	case twoFifths:
		widget1Width = containerSize.Width * 2 / 5
	case twoThirds:
		widget1Width = containerSize.Width * 2 / 3
	case threeFourths:
		widget1Width = containerSize.Width * 3 / 4
	case fourFifths:
		widget1Width = containerSize.Width * 4 / 5
	case threeFifths:
		widget1Width = containerSize.Width * 3 / 5
	}

	widget2Width := containerSize.Width - widget1Width

	s.widget1.Resize(fyne.NewSize(widget1Width, s.widget1.MinSize().Height))
	s.widget2.Resize(fyne.NewSize(widget2Width, s.widget2.MinSize().Height))

	// Calculate X positions based on alignment:
	var widget1X, widget2X float32
	switch s.alignment {
	case alignLeft:
		widget1X = 0
		widget2X = widget1Width
	case alignRight:
		widget1X = containerSize.Width - widget1Width - widget2Width
		widget2X = containerSize.Width - widget2Width
	case alignOpposed:
		widget1X = 0
		widget2X = containerSize.Width - widget2Width
	case alignCenter:
		widget1X = (containerSize.Width - widget1Width - widget2Width) / 2
		widget2X = widget1X + widget1Width
	}

	s.widget1.Move(fyne.NewPos(widget1X, 0))
	s.widget2.Move(fyne.NewPos(widget2X, 0))
}

// NewSplitRowWithAlignment creates a split row with specified alignment and proportion.
func NewSplitRowWithAlignment(widget1, widget2 fyne.CanvasObject, proportion FirstWidgetProportion, alignment Alignment) *fyne.Container {
	layout := &splitLayout{
		widget1:    widget1,
		widget2:    widget2,
		proportion: proportion,
		alignment:  alignment,
	}
	return container.New(layout, widget1, widget2)
}

// NewSplitRow creates a split row with default (left) alignment.
func NewSplitRow(widget1, widget2 fyne.CanvasObject, proportion FirstWidgetProportion) *fyne.Container {
	return NewSplitRowWithAlignment(widget1, widget2, proportion, alignLeft)
}
