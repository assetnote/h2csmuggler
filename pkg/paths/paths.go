package paths

import (
	"fmt"
	"net/url"

	"github.com/pkg/errors"
)

// Pitchfork will take the initial base, http://foo.com and append paths
// returning full URLs e.g. http://foo.com/1 http://foo.com/2
func Pitchfork(base string, paths []string) (ret []string, err error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse base url")
	}

	for _, p := range paths {
		parsed.Path = p
		ret = append(ret, parsed.String())
	}
	return
}

// Prefix will cross multiply the prefix and paths. If either is empty
// an empty slice will be returned
func Prefix(prefix []string, paths []string) (ret []string) {
	for _, pre := range prefix {
		for _, p := range paths {
			ret = append(ret, fmt.Sprintf("%s/%s", pre, p))
		}
	}
	return
}
