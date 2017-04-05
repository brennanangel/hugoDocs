// Copyright 2017-present The Hugo Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package output

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"reflect"

	"github.com/mitchellh/mapstructure"

	"github.com/spf13/hugo/media"
)

// Format represents an output representation, usually to a file on disk.
type Format struct {
	// The Name is used as an identifier. Internal output formats (i.e. HTML and RSS)
	// can be overridden by providing a new definition for those types.
	Name string

	MediaType media.Type

	// Must be set to a value when there are two or more conflicting mediatype for the same resource.
	Path string

	// The base output file name used when not using "ugly URLs", defaults to "index".
	BaseName string

	// The value to use for rel links
	//
	// See https://www.w3schools.com/tags/att_link_rel.asp
	//
	// AMP has a special requirement in this department, see:
	// https://www.ampproject.org/docs/guides/deploy/discovery
	// I.e.:
	// <link rel="amphtml" href="https://www.example.com/url/to/amp/document.html">
	Rel string

	// The protocol to use, i.e. "webcal://". Defaults to the protocol of the baseURL.
	Protocol string

	// IsPlainText decides whether to use text/template or html/template
	// as template parser.
	IsPlainText bool

	// IsHTML returns whether this format is int the HTML family. This includes
	// HTML, AMP etc. This is used to decide when to create alias redirects etc.
	IsHTML bool

	// Enable to ignore the global uglyURLs setting.
	NoUgly bool
}

var (
	// An ordered list of built-in output formats
	//
	// See https://www.ampproject.org/learn/overview/
	AMPFormat = Format{
		Name:      "AMP",
		MediaType: media.HTMLType,
		BaseName:  "index",
		Path:      "amp",
		Rel:       "amphtml",
		IsHTML:    true,
	}

	CalendarFormat = Format{
		Name:        "Calendar",
		MediaType:   media.CalendarType,
		IsPlainText: true,
		Protocol:    "webcal://",
		BaseName:    "index",
		Rel:         "alternate",
	}

	CSSFormat = Format{
		Name:        "CSS",
		MediaType:   media.CSSType,
		BaseName:    "styles",
		IsPlainText: true,
		Rel:         "stylesheet",
	}
	CSVFormat = Format{
		Name:        "CSV",
		MediaType:   media.CSVType,
		BaseName:    "index",
		IsPlainText: true,
		Rel:         "alternate",
	}

	HTMLFormat = Format{
		Name:      "HTML",
		MediaType: media.HTMLType,
		BaseName:  "index",
		Rel:       "canonical",
		IsHTML:    true,
	}

	JSONFormat = Format{
		Name:        "JSON",
		MediaType:   media.JSONType,
		BaseName:    "index",
		IsPlainText: true,
		Rel:         "alternate",
	}

	RSSFormat = Format{
		Name:      "RSS",
		MediaType: media.RSSType,
		BaseName:  "index",
		NoUgly:    true,
		Rel:       "alternate",
	}
)

var DefaultFormats = Formats{
	AMPFormat,
	CalendarFormat,
	CSSFormat,
	CSVFormat,
	HTMLFormat,
	JSONFormat,
	RSSFormat,
}

func init() {
	sort.Sort(DefaultFormats)
}

type Formats []Format

func (f Formats) Len() int           { return len(f) }
func (f Formats) Swap(i, j int)      { f[i], f[j] = f[j], f[i] }
func (f Formats) Less(i, j int) bool { return f[i].Name < f[j].Name }

// GetBySuffix gets a output format given as suffix, e.g. "html".
// It will return false if no format could be found, or if the suffix given
// is ambiguous.
// The lookup is case insensitive.
func (formats Formats) GetBySuffix(suffix string) (f Format, found bool) {
	for _, ff := range formats {
		if strings.EqualFold(suffix, ff.MediaType.Suffix) {
			if found {
				// ambiguous
				found = false
				return
			}
			f = ff
			found = true
		}
	}
	return
}

// GetByName gets a format by its identifier name.
func (formats Formats) GetByName(name string) (f Format, found bool) {
	for _, ff := range formats {
		if strings.EqualFold(name, ff.Name) {
			f = ff
			found = true
			return
		}
	}
	return
}

// GetByNames gets a list of formats given a list of identifiers.
func (formats Formats) GetByNames(names ...string) (Formats, error) {
	var types []Format

	for _, name := range names {
		tpe, ok := formats.GetByName(name)
		if !ok {
			return types, fmt.Errorf("OutputFormat with key %q not found", name)
		}
		types = append(types, tpe)
	}
	return types, nil
}

// FromFilename gets a Format given a filename.
func (formats Formats) FromFilename(filename string) (f Format, found bool) {
	// mytemplate.amp.html
	// mytemplate.html
	// mytemplate
	var ext, outFormat string

	parts := strings.Split(filename, ".")
	if len(parts) > 2 {
		outFormat = parts[1]
		ext = parts[2]
	} else if len(parts) > 1 {
		ext = parts[1]
	}

	if outFormat != "" {
		return formats.GetByName(outFormat)
	}

	if ext != "" {
		return formats.GetBySuffix(ext)
	}
	return
}

// DecodeFormats takes a list of output format configurations and merges those,
// in the order given, with the Hugo defaults as the last resort.
func DecodeFormats(mediaTypes media.Types, maps ...map[string]interface{}) (Formats, error) {
	f := make(Formats, len(DefaultFormats))
	copy(f, DefaultFormats)

	for _, m := range maps {
		for k, v := range m {
			found := false
			for i, vv := range f {
				if strings.EqualFold(k, vv.Name) {
					// Merge it with the existing
					if err := decode(mediaTypes, v, &f[i]); err != nil {
						return f, err
					}
					found = true
				}
			}
			if !found {
				var newOutFormat Format
				newOutFormat.Name = k
				if err := decode(mediaTypes, v, &newOutFormat); err != nil {
					return f, err
				}

				// We need values for these
				if newOutFormat.BaseName == "" {
					newOutFormat.BaseName = "index"
				}
				if newOutFormat.Rel == "" {
					newOutFormat.Rel = "alternate"
				}

				f = append(f, newOutFormat)
			}
		}
	}

	sort.Sort(f)

	return f, nil
}

func decode(mediaTypes media.Types, input, output interface{}) error {
	config := &mapstructure.DecoderConfig{
		Metadata:         nil,
		Result:           output,
		WeaklyTypedInput: true,
		DecodeHook: func(a reflect.Type, b reflect.Type, c interface{}) (interface{}, error) {
			if a.Kind() == reflect.Map {
				dataVal := reflect.Indirect(reflect.ValueOf(c))
				for _, key := range dataVal.MapKeys() {
					keyStr, ok := key.Interface().(string)
					if !ok {
						// Not a string key
						continue
					}
					if strings.EqualFold(keyStr, "mediaType") {
						// If mediaType is a string, look it up and replace it
						// in the map.
						vv := dataVal.MapIndex(key)
						if mediaTypeStr, ok := vv.Interface().(string); ok {
							mediaType, found := mediaTypes.GetByType(mediaTypeStr)
							if !found {
								return c, fmt.Errorf("media type %q not found", mediaTypeStr)
							}
							dataVal.SetMapIndex(key, reflect.ValueOf(mediaType))
						}
					}
				}
			}
			return c, nil
		},
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return err
	}

	return decoder.Decode(input)
}

func (f Format) BaseFilename() string {
	return f.BaseName + "." + f.MediaType.Suffix
}

func (f Format) MarshalJSON() ([]byte, error) {
	type Alias Format
	return json.Marshal(&struct {
		MediaType string
		Alias
	}{
		MediaType: f.MediaType.String(),
		Alias:     (Alias)(f),
	})
}
