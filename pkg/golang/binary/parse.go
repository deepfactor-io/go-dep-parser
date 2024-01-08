package binary

import (
	"bytes"
	"debug/buildinfo"
	"debug/elf"
	"fmt"
	"io"
	"strings"

	"golang.org/x/xerrors"

	dio "github.com/deepfactor-io/go-dep-parser/pkg/io"
	"github.com/deepfactor-io/go-dep-parser/pkg/types"
)

var (
	ErrUnrecognizedExe     = xerrors.New("unrecognized executable format")
	ErrNonGoBinary         = xerrors.New("non go binary")
	readSize               = 32 * 1024
	elfPrefix              = []byte("\x7fELF")
	elfGoNote              = []byte("Go\x00\x00")
	elfGNUNote             = []byte("GNU\x00")
	errProgramNotSupported = fmt.Errorf("Program not supported")
	errBuildIDNotFound     = fmt.Errorf("Go BuildID not found")
)

const offsetToNoteData = 16
const offsetToNoteFields = 12
const sizeOfNoteNameAndValue = 4
const elfGoBuildIDTag = 4
const gnuBuildIDTag = 3

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
	var buildID string

	info, err := buildinfo.Read(r)
	if err != nil {
		return nil, nil, convertError(err)
	}

	if len(info.Deps) > 0 {
		// get build id
		buildID, err = getBuildID(r)
		if err != nil {
			warnings = []string{err.Error()}
		}
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

		libs = append(libs, types.Library{
			Name:     mod.Path,
			Version:  mod.Version,
			BuildID:  buildID,
			Warnings: warnings,
		})
	}

	return libs, nil, nil
}

/**
 * The Go build ID is stored in a note described by an ELF PT_NOTE prog
 * header. The caller has already opened filename, to get f, and read
 * at least 4 kB out, in data.
 */
func readELF(r dio.ReadSeekerAt, data []byte) (buildid string, err error) {
	/*
	 * Assume the note content is in the data, already read.
	 * Rewrite the ELF header to set shoff and shnum to 0, so that we can pass
	 * the data to elf.NewFile and it will decode the Prog list but not
	 * try to read the section headers and the string table from disk.
	 * That's a waste of I/O when all we care about is the Prog list
	 * and the one ELF note.
	 * These specific bytes are at offsets 40-43, 44-47, 60, and 61 in the data
	 * slice.
	 */
	switch elf.Class(data[elf.EI_CLASS]) {
	case elf.ELFCLASS32:
		return "", errProgramNotSupported
	case elf.ELFCLASS64:
		data[40], data[41], data[42], data[43] = 0, 0, 0, 0
		data[44], data[45], data[46], data[47] = 0, 0, 0, 0
		data[60] = 0
		data[61] = 0
	}
	ef, err := elf.NewFile(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	var gnu string
	for _, p := range ef.Progs {
		if p.Type != elf.PT_NOTE || p.Filesz < offsetToNoteData {
			continue
		}
		var note []byte
		if p.Off+p.Filesz < uint64(len(data)) {
			note = data[p.Off : p.Off+p.Filesz]
		} else {
			/*
			 * For some linkers, such as the Solaris linker,
			 * the buildid may not be found in data (which
			 * likely contains the first 16kB of the file)
			 * or even the first few megabytes of the file
			 * due to differences in note segment placement;
			 * in that case, extract the note data manually.
			 */
			_, err = r.Seek(int64(p.Off), io.SeekStart)
			if err != nil {
				return "", err
			}
			note = make([]byte, p.Filesz)
			_, err = io.ReadFull(r, note)
			if err != nil {
				return "", err
			}
		}
		filesz := p.Filesz
		off := p.Off
		for filesz >= offsetToNoteData {
			nameSize := ef.ByteOrder.Uint32(note)
			valSize := ef.ByteOrder.Uint32(note[sizeOfNoteNameAndValue:])
			tag := ef.ByteOrder.Uint32(note[8:])
			nname := note[offsetToNoteFields : offsetToNoteFields+sizeOfNoteNameAndValue]
			if nameSize == sizeOfNoteNameAndValue && offsetToNoteData+valSize <= uint32(len(note)) &&
				tag == elfGoBuildIDTag && bytes.Equal(nname, elfGoNote) {
				return string(note[offsetToNoteData : offsetToNoteData+valSize]), nil
			}
			if nameSize == sizeOfNoteNameAndValue && offsetToNoteData+valSize <= uint32(len(note)) &&
				tag == gnuBuildIDTag && bytes.Equal(nname, elfGNUNote) {
				gnu = string(note[offsetToNoteData : offsetToNoteData+valSize])
			}
			nameSize = (nameSize + 3) &^ 3
			valSize = (valSize + 3) &^ 3
			notesz := uint64(offsetToNoteFields + nameSize + valSize)
			if filesz <= notesz {
				break
			}
			off += notesz
			align := p.Align
			if align != 0 {
				alignedOff := (off + align - 1) &^ (align - 1)
				notesz += alignedOff - off
				off = alignedOff
			}
			filesz -= notesz
			note = note[notesz:]
		}
	}
	/*
	 * If we didn't find a Go note, use a GNU note if available.
	 * This is what gccgo uses.
	 */
	if gnu != "" {
		return gnu, nil
	}
	/* No note. Treat as successful but build ID empty. */
	return "", nil
}

func getBuildID(r dio.ReadSeekerAt) (id string, err error) {
	/*
	 * Adding some sanity check
	 * we only support elf header
	 */

	buf := make([]byte, 8)
	if _, err := r.ReadAt(buf, 0); err != nil {
		return "", err
	}
	if string(buf) != "!<arch>\n" {
		if string(buf) == "<bigaf>\n" {
			return "", errProgramNotSupported
		}
		data := make([]byte, readSize)
		_, err = io.ReadFull(r, data)
		if err == io.ErrUnexpectedEOF {
			err = nil
		}
		if err != nil {
			return "", err
		}
		if bytes.HasPrefix(data, elfPrefix) {
			return readELF(r, data)
		}
	}
	return "", errProgramNotSupported
}
