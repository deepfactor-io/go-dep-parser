package binary

import (
	"debug/buildinfo"
	"debug/elf"
	"fmt"
	"strings"

	"golang.org/x/xerrors"

	dio "github.com/deepfactor-io/go-dep-parser/pkg/io"
	"github.com/deepfactor-io/go-dep-parser/pkg/types"
)

var (
	ErrUnrecognizedExe = xerrors.New("unrecognized executable format")
	ErrNonGoBinary     = xerrors.New("non go binary")
)

// convertError detects buildinfo.errUnrecognizedFormat and convert to
// ErrUnrecognizedExe and convert buildinfo.errNotGoExe to ErrNonGoBinary
func convertError(err error) error {
	errText := err.Error()
	if strings.HasSuffix(errText, "unrecognized file format") {
		return ErrUnrecognizedExe
	}
	if strings.HasSuffix(errText, "not a Go executable") {
		return ErrNonGoBinary
	}

	return err
}

type Parser struct{}

func NewParser() types.Parser {
	return &Parser{}
}

// Parse scans file to try to report the Go and module versions.
func (p *Parser) Parse(r dio.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	var warnings []string
	var checkBuildID bool
	var buildID string

	info, err := buildinfo.Read(r)
	if err != nil {
		return nil, nil, convertError(err)
	}

	libs := make([]types.Library, 0, len(info.Deps))

	for _, dep := range info.Deps {
		// binaries with old go version may incorrectly add module in Deps
		// In this case Path == "", Version == "Devel"
		// we need to skip this
		if dep.Path == "" {
			continue
		}

		mod := dep
		if dep.Replace != nil {
			mod = dep.Replace
		}

		if !checkBuildID {
			// get build id
			buildID, err = getBuildID(r)
			if err != nil {
				warnings = []string{err.Error()}
			}
			checkBuildID = true
		}

		libs = append(libs, types.Library{
			Name:     mod.Path,
			Version:  mod.Version,
			BuildID:  buildID,
			Warnings: warnings,
		})
	}

	return libs, nil, nil
}

func getBuildID(r dio.ReadSeekerAt) (string, error) {
	file, err := elf.NewFile(r)
	if err != nil {
		return "", err
	}
	defer file.Close()

	for _, section := range file.Sections {
		if section.Name == ".note.go.buildid" {
			data, _ := section.Data()
			return string(data[16:]), nil
		}
	}
	return "", fmt.Errorf("Go BuildID not found")
}
