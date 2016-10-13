package main

import (
	"fmt"
)

var Version = &NxVersion{
	Major: 1,
	Minor: 0,
	Patch: 1,
}

type NxVersion struct {
	Major int
	Minor int
	Patch int
}

func (v *NxVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}
