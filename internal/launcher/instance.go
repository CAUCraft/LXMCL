package launcher

import (
	"github.com/CAUCraft/LXMCL/internal/instance"
)

const (
	JavaInherit       = instance.JavaInherit
	JavaAuto          = instance.JavaAuto
	JavaCustom        = instance.JavaCustom
	DisplayDefault    = instance.DisplayDefault
	DisplayResolution = instance.DisplayResolution
	DisplayFullscreen = instance.DisplayFullscreen
)

type LaunchOptions = instance.LaunchOptions

func ValidateSettings(s Settings) error { return instance.ValidateSettings(s) }

const (
	LoaderVanilla  = instance.LoaderVanilla
	LoaderFabric   = instance.LoaderFabric
	LoaderForge    = instance.LoaderForge
	LoaderNeoForge = instance.LoaderNeoForge
	LoaderQuilt    = instance.LoaderQuilt
)
