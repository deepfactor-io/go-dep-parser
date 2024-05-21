package types

import "io/fs"

type PackageManifest interface {
	// Pkg ID is formed using Pkg Name and version
	PackageID() string
	// Declared license with the package manifest
	DeclaredLicense() string
}

type PackageManifestParser interface {
	ParseManifest(fsys fs.FS, path string) (PackageManifest, error)
}
