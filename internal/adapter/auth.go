package adapter

import "github.com/xifan2333/webcast-mate/internal/secrets"

// AuthFromSecrets copies secrets buckets into stdout AuthBuckets.
func AuthFromSecrets(f *secrets.File) AuthBuckets {
	if f == nil {
		return AuthBuckets{}
	}
	c, h, p := f.AuthMaps()
	return AuthBuckets{Cookies: c, Headers: h, Params: p}
}
