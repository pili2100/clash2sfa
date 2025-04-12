package handle

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"log/slog"

	"github.com/Masterminds/semver/v3"
	"github.com/xmdhs/clash2sfa/model"
	"github.com/xmdhs/clash2sfa/service"
	"github.com/xmdhs/clash2sfa/utils"
)

type Handle struct {
	convert  *service.Convert
	l        *slog.Logger
	configFs fs.FS
}

func NewHandle(convert *service.Convert, l *slog.Logger, configFs fs.FS) *Handle {
	return &Handle{
		convert:  convert,
		l:        l,
		configFs: configFs,
	}
}

func Frontend(frontendByte []byte, age int) http.HandlerFunc {
	sage := strconv.Itoa(age)
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age="+sage)
		w.Write(frontendByte)
	}
}

func (h *Handle) Sub(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	config := r.FormValue("config")
	curl := r.FormValue("configurl")
	sub := r.FormValue("sub")
	include := r.FormValue("include")
	exclude := r.FormValue("exclude")
	addTag := r.FormValue("addTag")
	disableUrlTest := r.FormValue("disableUrlTest")
	outFields := r.FormValue("outFields")
	disableUrlTestb := false
	addTagb := false

	if sub == "" {
		h.l.DebugContext(ctx, "sub 不得为空")
		http.Error(w, "sub 不得为空", 400)
		return
	}
	if addTag == "true" {
		addTagb = true
	}
	if disableUrlTest == "true" {
		disableUrlTestb = true
	}
	a := model.ConvertArg{
		Sub:            sub,
		Include:        include,
		Exclude:        exclude,
		ConfigUrl:      curl,
		AddTag:         addTagb,
		DisableUrlTest: disableUrlTestb,
		OutFields:      true,
	}

	var defaultConfig []byte

	v := utils.GetSingBoxVersion(r)
	if v == nil || v.GreaterThan(semver.MustParse("1.10.99")) {
		a.OutFields = false
		defaultConfig = utils.FsReadAll(h.configFs, "config.json-1.11.0+.template")
	} else {
		defaultConfig = utils.FsReadAll(h.configFs, "config.json.template")
	}
	if outFields == "0" {
		a.OutFields = false
	}
	if outFields == "1" {
		a.OutFields = true
	}

	if a.ConfigUrl != "" && !strings.HasPrefix(a.ConfigUrl, "http") {
		b, err := func() ([]byte, error) {
			f, err := h.configFs.Open(a.ConfigUrl)
			if err != nil {
				return nil, err
			}
			b, err := io.ReadAll(f)
			if err != nil {
				return nil, err
			}
			return b, nil
		}()
		if err != nil {
			h.l.WarnContext(ctx, err.Error())
			http.Error(w, err.Error(), 400)
			return
		}
		a.Config = string(b)
		a.ConfigUrl = ""
	}

	rc := http.NewResponseController(w)
	rc.SetWriteDeadline(time.Now().Add(2 * time.Minute))

	b, err := func() ([]byte, error) {
		if config != "" {
			b, err := zlibDecode(config)
			if err != nil {
				return nil, err
			}
			a.Config = string(b)
		}
		return h.convert.MakeConfig(ctx, a, defaultConfig)
	}()
	if err != nil {
		h.l.WarnContext(ctx, err.Error())
		http.Error(w, err.Error(), 500)
		return
	}
	w.Write(b)

}

func zlibDecode(s string) ([]byte, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	r, err := zlib.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	b, err = io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return b, nil
}
