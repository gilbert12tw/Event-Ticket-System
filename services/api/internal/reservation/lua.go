package reservation

import _ "embed"

//go:embed reserve.lua
var reserveScript string

//go:embed release.lua
var releaseScript string

//go:embed commit.lua
var commitScript string
