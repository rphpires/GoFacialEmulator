package devicelog

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	maxBody   = 2048    // corpo gravado, depois de sanitizar
	maxCopy   = 2 << 20 // acima disto o corpo da requisição não é copiado
	maxFields = 6
	maxValue  = 40
	cutSuffix = " …(cortado)"
)

// keyFields são os campos que identificam o objeto da operação nos
// protocolos Hikvision (ISAPI) e Dahua (CGI). Comparação sem maiúsculas.
var keyFields = map[string]bool{
	"employeeno": true, "employeenostring": true, "name": true, "cardno": true,
	"userid": true, "cardname": true, "usertype": true, "major": true,
	"minor": true, "majoreventtype": true, "subeventtype": true, "serialno": true,
	"eventtype": true, "code": true, "searchid": true, "facelibtype": true,
	"fpid": true, "action": true, "recno": true, "status": true,
}

var (
	reBase64     = regexp.MustCompile(`[A-Za-z0-9+/=]{257,}`)
	reXMLPass    = regexp.MustCompile(`(?is)<(password|pwd)>[^<]*</(password|pwd)>`)
	reJSONPass   = regexp.MustCompile(`(?i)"(password|pwd)"\s*:\s*"[^"]*"`)
	reFormPass   = regexp.MustCompile(`(?i)\b(password|pwd)=[^&\s]*`)
	reXMLBetween = regexp.MustCompile(`>\s+<`)
	reSpaces     = regexp.MustCompile(`\s+`)
	reXMLField   = regexp.MustCompile(`<([A-Za-z]\w*)>([^<]{0,200})</([A-Za-z]\w*)>`)
	reDigestUser = regexp.MustCompile(`(?i)username="([^"]*)"`)
)

// Describe devolve o corpo em uma linha, sem imagens, base64 e senhas,
// cortado em maxBody bytes.
func Describe(contentType string, body []byte) string {
	if len(body) == 0 {
		return ""
	}
	mt, params := mediaType(contentType, body)
	var s string
	switch {
	case strings.HasPrefix(mt, "multipart/"):
		s = describeMultipart(body, params["boundary"])
	case isBinary(mt, body):
		if mt == "" {
			mt = "application/octet-stream"
		}
		s = fmt.Sprintf("<%s %s>", mt, humanSize(len(body)))
	default:
		s = cleanText(string(body))
	}
	return truncate(s, maxBody)
}

// mediaType lê o Content-Type ou, sem ele, reconhece um multipart pelo
// "--boundary" da primeira linha (formato dos streams e do push).
func mediaType(contentType string, body []byte) (string, map[string]string) {
	if mt, params, err := mime.ParseMediaType(contentType); err == nil {
		return mt, params
	}
	t := bytes.TrimLeft(body, "\r\n \t")
	if bytes.HasPrefix(t, []byte("--")) {
		linha := t[2:]
		if i := bytes.IndexAny(linha, "\r\n"); i >= 0 {
			linha = linha[:i]
		}
		return "multipart/mixed", map[string]string{"boundary": string(linha)}
	}
	return "", nil
}

func isBinary(mt string, body []byte) bool {
	for _, p := range []string{"image/", "video/", "audio/", "application/octet-stream"} {
		if strings.HasPrefix(mt, p) {
			return true
		}
	}
	return !utf8.Valid(body)
}

// eachPart percorre as partes de um multipart. Tolera stream sem boundary
// final: usa o que conseguiu ler da última parte.
func eachPart(body []byte, boundary string, fn func(name, ct string, data []byte)) bool {
	if boundary == "" {
		return false
	}
	r := multipart.NewReader(bytes.NewReader(bytes.TrimLeft(body, "\r\n \t")), boundary)
	achou := false
	for {
		p, err := r.NextPart()
		if err != nil {
			return achou
		}
		data, _ := io.ReadAll(p)
		achou = true
		fn(p.FormName(), p.Header.Get("Content-Type"), data)
	}
}

func describeMultipart(body []byte, boundary string) string {
	var parts []string
	ok := eachPart(body, boundary, func(name, ct string, data []byte) {
		if name == "" {
			name = "parte"
		}
		parts = append(parts, "["+name+"] "+Describe(ct, data))
	})
	if !ok {
		return fmt.Sprintf("<multipart %s>", humanSize(len(body)))
	}
	return strings.Join(parts, " ")
}

func cleanText(s string) string {
	s = reXMLPass.ReplaceAllString(s, "<$1>***</$2>")
	s = reJSONPass.ReplaceAllString(s, `"$1":"***"`)
	s = reFormPass.ReplaceAllString(s, "$1=***")
	s = reBase64.ReplaceAllStringFunc(s, func(m string) string {
		return "<base64 " + humanSize(len(m)*3/4) + ">"
	})
	s = reXMLBetween.ReplaceAllString(s, "><")
	s = reSpaces.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	corte := max
	for corte > 0 && !utf8.RuneStart(s[corte]) {
		corte--
	}
	return s[:corte] + cutSuffix
}

func humanSize(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1024:
		return fmt.Sprintf("%dKB", (n+512)/1024)
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// KeyFields extrai os campos que identificam o objeto da operação: primeiro
// da query, depois do corpo (JSON, XML, form ou multipart).
func KeyFields(contentType string, body []byte, query url.Values) string {
	c := &collector{seen: map[string]bool{}}
	c.addValues(query)
	c.fromBody(contentType, body)
	return strings.Join(c.out, " ")
}

type collector struct {
	seen map[string]bool
	out  []string
}

func (c *collector) add(k, v string) {
	lk := strings.ToLower(k)
	if !keyFields[lk] || c.seen[lk] || len(c.out) >= maxFields {
		return
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return
	}
	if utf8.RuneCountInString(v) > maxValue {
		v = string([]rune(v)[:maxValue]) + "…"
	}
	if strings.ContainsAny(v, " \"=") {
		v = strconv.Quote(v)
	}
	c.seen[lk] = true
	c.out = append(c.out, k+"="+v)
}

func (c *collector) addValues(v url.Values) {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		c.add(k, v.Get(k))
	}
}

func (c *collector) fromBody(contentType string, body []byte) {
	if len(body) == 0 || len(body) > maxCopy {
		return
	}
	mt, params := mediaType(contentType, body)
	switch {
	case strings.HasPrefix(mt, "multipart/"):
		ok := eachPart(body, params["boundary"], func(_, ct string, data []byte) {
			if !isBinary(ct, data) {
				c.fromBody(ct, data)
			}
		})
		if !ok {
			c.fromLooseJSON(body)
		}
	case mt == "application/x-www-form-urlencoded":
		if v, err := url.ParseQuery(string(body)); err == nil {
			c.addValues(v)
		}
	default:
		t := bytes.TrimSpace(body)
		if len(t) == 0 {
			return
		}
		if t[0] == '{' || t[0] == '[' {
			var v interface{}
			if json.Unmarshal(t, &v) == nil {
				c.walkJSON(v)
			}
			return
		}
		if t[0] == '<' {
			for _, m := range reXMLField.FindAllStringSubmatch(string(t), -1) {
				if m[1] == m[3] {
					c.add(m[1], m[2])
				}
			}
		}
	}
}

// fromLooseJSON é o último recurso para uma part de stream que o leitor de
// multipart não conseguiu abrir: tenta o trecho entre o primeiro '{' e o
// último '}'.
func (c *collector) fromLooseJSON(body []byte) {
	i, j := bytes.IndexByte(body, '{'), bytes.LastIndexByte(body, '}')
	if i < 0 || j <= i {
		return
	}
	var v interface{}
	if json.Unmarshal(body[i:j+1], &v) == nil {
		c.walkJSON(v)
	}
}

// walkJSON visita primeiro os escalares de cada nível (em ordem alfabética),
// depois desce nos objetos e listas.
func (c *collector) walkJSON(v interface{}) {
	switch x := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			switch val := x[k].(type) {
			case string:
				c.add(k, val)
			case float64:
				c.add(k, strconv.FormatFloat(val, 'f', -1, 64))
			case bool:
				c.add(k, strconv.FormatBool(val))
			}
		}
		for _, k := range keys {
			switch x[k].(type) {
			case map[string]interface{}, []interface{}:
				c.walkJSON(x[k])
			}
		}
	case []interface{}:
		for _, item := range x {
			c.walkJSON(item)
		}
	}
}

// UserFromAuth tira só o usuário do header Authorization (Digest ou Basic).
// Nunca devolve senha nem o header inteiro.
func UserFromAuth(header string) string {
	lower := strings.ToLower(header)
	switch {
	case strings.HasPrefix(lower, "digest "):
		if m := reDigestUser.FindStringSubmatch(header); m != nil {
			return m[1]
		}
	case strings.HasPrefix(lower, "basic "):
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(header[6:]))
		if err == nil {
			u, _, _ := strings.Cut(string(raw), ":")
			return u
		}
	}
	return ""
}
