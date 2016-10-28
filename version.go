package main

import (
	"fmt"
)

var Version = &NxVersion{
	Major: 1,
	Minor: 3,
	Patch: 0,
}

type NxVersion struct {
	Major int
	Minor int
	Patch int
}

func (v *NxVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}
