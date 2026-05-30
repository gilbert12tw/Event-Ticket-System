package reservation

import _ "embed"

//go:embed reserve.lua
var reserveScript string

//go:embed release.lua
var releaseScript string

//go:embed commit.lua
var commitScript string

//go:embed compensation_release.lua
var compensationReleaseScript string

//go:embed compensation_drop.lua
var compensationDropScript string

//go:embed compensation_cap.lua
var compensationCapScript string
