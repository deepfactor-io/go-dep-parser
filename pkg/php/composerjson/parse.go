package composerjson

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"golang.org/x/exp/maps"
	"golang.org/x/xerrors"

	dio "github.com/deepfactor-io/go-dep-parser/pkg/io"
	"github.com/deepfactor-io/go-dep-parser/pkg/types"
)

type composerJSON struct {
	Require    map[string]string `json:"require"`
	RequireDev map[string]string `json:"require-dev"`
}

type Parser struct{}

func NewParser() types.Parser {
	return &Parser{}
}

func (p *Parser) Parse(r dio.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	var cJSON composerJSON
	input, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, xerrors.Errorf("read error: %w", err)
	}
	if err = json.Unmarshal(input, &cJSON); err != nil {
		return nil, nil, xerrors.Errorf("unmarshal error: %w", err)
	}

	libs := map[string]types.Library{}

	for pkg, ver := range cJSON.Require {
		lib := types.Library{
			ID:       pkg,
			Name:     pkg,
			Version:  ver,
			Indirect: false,
			Dev:      false,
		}
		libs[lib.Name+fmt.Sprint(lib.Dev)] = lib
	}

	for pkg, ver := range cJSON.RequireDev {
		lib := types.Library{
			ID:       pkg,
			Name:     pkg,
			Version:  ver,
			Indirect: false,
			Dev:      true,
		}
		libs[lib.Name+fmt.Sprint(lib.Dev)] = lib
	}
	libSlice := maps.Values(libs)
	sort.Sort(types.Libraries(libSlice))

	return libSlice, []types.Dependency{}, nil
}
