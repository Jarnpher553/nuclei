package authprovider

import (
	"net"
	"net/url"
	"regexp"
	"strings"

	"github.com/Jarnpher553/nuclei/v3/pkg/authprovider/authx"
	errorutil "github.com/projectdiscovery/utils/errors"
	urlutil "github.com/projectdiscovery/utils/url"
)

// FileAuthProvider is an auth provider for file based auth
// it accepts a secrets file and returns its provider
type FileAuthProvider struct {
	Path     string
	store    *authx.Authx
	compiled map[*regexp.Regexp]authx.AuthStrategy
	domains  map[string]authx.AuthStrategy
}

// NewFileAuthProvider creates a new file based auth provider
func NewFileAuthProvider(path string, callback authx.LazyFetchSecret) (AuthProvider, error) {
	store, err := authx.GetAuthDataFromFile(path)
	if err != nil {
		return nil, err
	}
	if len(store.Secrets) == 0 && len(store.Dynamic) == 0 {
		return nil, ErrNoSecrets
	}
	if len(store.Dynamic) > 0 && callback == nil {
		return nil, errorutil.New("lazy fetch callback is required for dynamic secrets")
	}
	for _, secret := range store.Secrets {
		if err := secret.Validate(); err != nil {
			return nil, errorutil.NewWithErr(err).Msgf("invalid secret in file: %s", path)
		}
	}
	for i, dynamic := range store.Dynamic {
		if err := dynamic.Validate(); err != nil {
			return nil, errorutil.NewWithErr(err).Msgf("invalid dynamic in file: %s", path)
		}
		dynamic.SetLazyFetchCallback(callback)
		store.Dynamic[i] = dynamic
	}
	f := &FileAuthProvider{Path: path, store: store}
	f.init()
	return f, nil
}

// init initializes the file auth provider
func (f *FileAuthProvider) init() {
	for _, secret := range f.store.Secrets {
		if len(secret.DomainsRegex) > 0 {
			for _, domain := range secret.DomainsRegex {
				if f.compiled == nil {
					f.compiled = make(map[*regexp.Regexp]authx.AuthStrategy)
				}
				compiled, err := regexp.Compile(domain)
				if err != nil {
					continue
				}
				f.compiled[compiled] = secret.GetStrategy()
			}
		}
		for _, domain := range secret.Domains {
			if f.domains == nil {
				f.domains = make(map[string]authx.AuthStrategy)
			}
			f.domains[strings.TrimSpace(domain)] = secret.GetStrategy()
			if strings.HasSuffix(domain, ":80") {
				f.domains[strings.TrimSuffix(domain, ":80")] = secret.GetStrategy()
			}
			if strings.HasSuffix(domain, ":443") {
				f.domains[strings.TrimSuffix(domain, ":443")] = secret.GetStrategy()
			}
		}
	}
	for _, dynamic := range f.store.Dynamic {
		if len(dynamic.DomainsRegex) > 0 {
			for _, domain := range dynamic.DomainsRegex {
				if f.compiled == nil {
					f.compiled = make(map[*regexp.Regexp]authx.AuthStrategy)
				}
				compiled, err := regexp.Compile(domain)
				if err != nil {
					continue
				}
				f.compiled[compiled] = &authx.DynamicAuthStrategy{Dynamic: dynamic}
			}
		}
		for _, domain := range dynamic.Domains {
			if f.domains == nil {
				f.domains = make(map[string]authx.AuthStrategy)
			}
			f.domains[strings.TrimSpace(domain)] = &authx.DynamicAuthStrategy{Dynamic: dynamic}
			if strings.HasSuffix(domain, ":80") {
				f.domains[strings.TrimSuffix(domain, ":80")] = &authx.DynamicAuthStrategy{Dynamic: dynamic}
			}
			if strings.HasSuffix(domain, ":443") {
				f.domains[strings.TrimSuffix(domain, ":443")] = &authx.DynamicAuthStrategy{Dynamic: dynamic}
			}
		}
	}
}

// LookupAddr looks up a given domain/address and returns appropriate auth strategy
func (f *FileAuthProvider) LookupAddr(addr string) authx.AuthStrategy {
	if strings.Contains(addr, ":") {
		// default normalization for host:port
		host, port, err := net.SplitHostPort(addr)
		if err == nil && (port == "80" || port == "443") {
			addr = host
		}
	}
	for domain, strategy := range f.domains {
		if strings.EqualFold(domain, addr) {
			return strategy
		}
	}
	for compiled, strategy := range f.compiled {
		if compiled.MatchString(addr) {
			return strategy
		}
	}
	return nil
}

// LookupURL looks up a given URL and returns appropriate auth strategy
func (f *FileAuthProvider) LookupURL(u *url.URL) authx.AuthStrategy {
	return f.LookupAddr(u.Host)
}

// LookupURLX looks up a given URL and returns appropriate auth strategy
func (f *FileAuthProvider) LookupURLX(u *urlutil.URL) authx.AuthStrategy {
	return f.LookupAddr(u.Host)
}

// GetTemplatePaths returns the template path for the auth provider
func (f *FileAuthProvider) GetTemplatePaths() []string {
	res := []string{}
	for _, dynamic := range f.store.Dynamic {
		if dynamic.TemplatePath != "" {
			res = append(res, dynamic.TemplatePath)
		}
	}
	return res
}

// PreFetchSecrets pre-fetches the secrets from the auth provider
func (f *FileAuthProvider) PreFetchSecrets() error {
	for _, s := range f.domains {
		if val, ok := s.(*authx.DynamicAuthStrategy); ok {
			if err := val.Dynamic.Fetch(false); err != nil {
				return err
			}
		}
	}
	for _, s := range f.compiled {
		if val, ok := s.(*authx.DynamicAuthStrategy); ok {
			if err := val.Dynamic.Fetch(false); err != nil {
				return err
			}
		}
	}
	return nil
}
