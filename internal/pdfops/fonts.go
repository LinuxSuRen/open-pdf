package pdfops

import _ "embed"

// Embedded Noto Sans SC (SIL Open Font License 1.1, see assets/fonts/OFL.txt).
// gopdf subsets fonts on write, so documents stay small regardless of these
// ~10 MB sources.
//
//go:embed assets/fonts/NotoSansSC-Regular.ttf
var mdFontRegular []byte

//go:embed assets/fonts/NotoSansSC-Bold.ttf
var mdFontBold []byte
