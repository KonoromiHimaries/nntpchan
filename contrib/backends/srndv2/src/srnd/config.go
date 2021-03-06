//
// config.go
//

package srnd

import (
	"bufio"
	"bytes"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"github.com/majestrate/configparser"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type FilterConfig struct {
	globalFilters []*regexp.Regexp
}

func (fc *FilterConfig) LoadFile(fname string) (err error) {
	var data []byte
	data, err = ioutil.ReadFile(fname)
	if err == nil {
		r := bytes.NewReader(data)
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			txt := sc.Text()
			idx := strings.Index(txt, "#")
			if idx >= 0 {
				txt = txt[:idx]
			}
			fc.globalFilters = append(fc.globalFilters, regexp.MustCompile(txt))
		}

	}
	return
}

type FeedConfig struct {
	policy           FeedPolicy
	quarks           map[string]string
	Addr             string
	sync             bool
	proxy_type       string
	proxy_addr       string
	username         string
	passwd           string
	linkauth_keyfile string
	tls_off          bool
	Name             string
	sync_interval    time.Duration
	connections      int
	disable          bool
}

type SpamConfig struct {
	enabled bool
	addr    string
}

type APIConfig struct {
	srndAddr     string
	frontendAddr string
}

type CryptoConfig struct {
	privkey_file string
	cert_file    string
	hostname     string
	cert_dir     string
}

type ThumbnailConfig struct {
	rules       []ThumbnailRule
	placeholder string
}

// pprof settings
type ProfilingConfig struct {
	bind   string
	enable bool
}

type SRNdConfig struct {
	daemon        map[string]string
	crypto        *CryptoConfig
	store         map[string]string
	database      map[string]string
	cache         map[string]string
	feeds         []FeedConfig
	frontend      map[string]string
	system        map[string]string
	worker        map[string]string
	pprof         *ProfilingConfig
	hooks         []*HookConfig
	inboundPolicy *FeedPolicy
	filter        FilterConfig
	spamconf      SpamConfig
	thumbnails    *ThumbnailConfig
}

// check for config files
// generate defaults on demand
func CheckConfig() {
	if !CheckFile("srnd.ini") {
		log.Println("No srnd.ini file found in working directory...")
		if !CheckFile(os.Getenv("SRND_INI_PATH")) {
			log.Printf("No config file found at %s...", os.Getenv("SRND_INI_PATH"))
			var conf *configparser.Configuration
			if InstallerEnabled() {
				res := make(chan *configparser.Configuration)
				installer := NewInstaller(res)
				go installer.Start()
				conf = <-res
				installer.Stop()
				close(res)
			} else {
				log.Println("Creating srnd.ini in working directory...")
				conf = GenSRNdConfig()
			}
			err := configparser.Save(conf, "srnd.ini")
			if err != nil {
				log.Fatal("cannot generate srnd.ini", err)
			}
		}
		if !CheckFile("feeds.ini") {
			if !CheckFile(os.Getenv("SRND_FEEDS_INI_PATH")) {
				log.Println("no feeds.ini, creating...")
				err := GenFeedsConfig()
				if err != nil {
					log.Fatal("cannot generate feeds.ini", err)
				}
			}
		}
	}
}

// generate default feeds.ini
func GenFeedsConfig() error {
	conf := configparser.NewConfiguration()

	sect := conf.NewSection("global")
	sect.Add("overchan.overchan", "1")
	sect.Add("ctl", "1")
	sect.Add("*", "0")

	sect = conf.NewSection("feed-2hu")
	sect.Add("proxy-type", "None")
	sect.Add("proxy-host", "127.0.0.1")
	sect.Add("proxy-port", "9050")
	sect.Add("host", "2hu-ch.org")
	sect.Add("port", "119")
	sect.Add("connections", "0")
	sect.Add("sync", "1")
	sect.Add("disable", "1")

	sect = conf.NewSection("2hu")
	sect.Add("overchan.overchan", "1")
	sect.Add("ctl", "1")
	sect.Add("*", "0")

	return configparser.Save(conf, "feeds.ini")
}

// generate default srnd.ini
func GenSRNdConfig() *configparser.Configuration {
	conf := configparser.NewConfiguration()

	// nntp related section
	sect := conf.NewSection("nntp")
	sect.Add("instance_name", "test.srndv2.tld")
	sect.Add("bind", ":1199")
	sect.Add("sync_on_start", "1")
	sect.Add("allow_anon", "0")
	sect.Add("allow_anon_attachments", "0")
	sect.Add("allow_attachments", "1")
	sect.Add("require_tls", "1")
	sect.Add("anon_nntp", "0")
	sect.Add("feeds", filepath.Join(".", "feeds.d"))
	sect.Add("archive", "0")
	sect.Add("article_lifetime", "0")
	sect.Add("filters_file", "filters.txt")
	sect.Add("secretkey", hex.EncodeToString(randbytes(32)))

	// spamd settings
	sect = conf.NewSection("spamd")
	sect.Add("enable", "0")
	sect.Add("addr", "127.0.0.1:783")

	// profiling settings
	sect = conf.NewSection("pprof")
	sect.Add("enable", "0")
	sect.Add("bind", "127.0.0.1:17000")

	// dummy hook
	sect = conf.NewSection("hook-dummy")
	sect.Add("enable", "0")
	sect.Add("exec", "/bin/true")

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "!!please-manually-set-this"
	}

	// crypto related section
	sect = conf.NewSection("crypto")
	sect.Add("tls-keyname", "overchan")
	sect.Add("tls-hostname", hostname)
	sect.Add("tls-trust-dir", "certs")

	// article store section
	sect = conf.NewSection("articles")

	sect.Add("store_dir", "articles")
	sect.Add("incoming_dir", "articles/tmp")
	sect.Add("attachments_dir", "webroot/img")
	sect.Add("thumbs_dir", "webroot/thm")
	sect.Add("convert_bin", "/usr/bin/convert")
	sect.Add("ffmpegthumbnailer_bin", "/usr/bin/ffmpeg")
	sect.Add("sox_bin", "/usr/bin/sox")
	sect.Add("identify_bin", "/usr/bin/identify")
	sect.Add("placeholder_thumbnail", "contrib/static/placeholder.png")
	sect.Add("compression", "0")

	// thumbnailing section
	sect = conf.NewSection("thumbnails")
	sect.Add("image/*", "{{.convert}} -thumbnail 200 {{.infile}} {{.outfile}}")
	sect.Add("image/gif", "{{.convert}} -thumbnail 200 {{.infile}}[0] {{.outfile}}")
	sect.Add("audio/*", "{{.ffmpeg}} -i {{.infile}} -an -vcodec copy {{.outfile}}")
	sect.Add("video/*", "{{.ffmpeg}} -i {{.infile}} -vf scale=300:200 -vframes 1 {{.outfile}}")
	sect.Add("*", "cp {{.placeholder}} {{.outfile}}")

	// database backend config
	sect = conf.NewSection("database")

	sect.Add("type", "postgres")
	sect.Add("schema", "srnd")
	sect.Add("host", "/var/run/postgresql")
	sect.Add("port", "")
	sect.Add("user", "")
	sect.Add("password", "")
	sect.Add("maxconns", "10")
	sect.Add("connlife", "10")
	sect.Add("connidle", "10")

	// cache backend config
	sect = conf.NewSection("cache")
	// defaults to null
	sect.Add("type", "null")

	// baked in static html frontend
	sect = conf.NewSection("frontend")
	sect.Add("enable", "1")
	sect.Add("allow_files", "1")
	sect.Add("regen_on_start", "0")
	sect.Add("regen_threads", "2")
	sect.Add("board_creation", "1")
	sect.Add("bind", "[::]:18000")
	sect.Add("name", "web.srndv2.test")
	sect.Add("webroot", "webroot")
	sect.Add("minimize_html", "0")
	sect.Add("prefix", "/")
	sect.Add("static_files", "contrib")
	sect.Add("templates", "contrib/templates/placebo")
	sect.Add("translations", "contrib/translations")
	sect.Add("markup_script", "contrib/lua/memeposting.lua")
	sect.Add("locale", "en")
	sect.Add("domain", "localhost")
	sect.Add("json-api", "0")
	sect.Add("json-api-username", "fucking-change-this-value")
	sect.Add("json-api-password", "seriously-fucking-change-this-value")
	secret_bytes := randbytes(8)
	secret := base32.StdEncoding.EncodeToString(secret_bytes)
	sect.Add("api-secret", secret)
	sect.Add("rapeme", "no")
	return conf
}

// save a list of feeds to overwrite feeds.ini
func SaveFeeds(feeds []FeedConfig, inboundPolicy *FeedPolicy) (err error) {
	conf := configparser.NewConfiguration()

	if inboundPolicy != nil {
		s := conf.NewSection("")
		for k, v := range inboundPolicy.rules {
			s.Add(k, v)
		}
	}

	for _, feed := range feeds {
		if len(feed.Name) == 0 {
			// don't do feed with no name
			continue
		}
		sect := conf.NewSection("feed-" + feed.Name)
		if len(feed.proxy_type) > 0 {
			sect.Add("proxy-type", feed.proxy_type)
		}
		phost, pport, _ := net.SplitHostPort(feed.proxy_addr)
		sect.Add("proxy-host", phost)
		sect.Add("proxy-port", pport)
		host, port, _ := net.SplitHostPort(feed.Addr)
		sect.Add("host", host)
		sect.Add("port", port)
		sync := "0"
		if feed.sync {
			sync = "1"
		}
		sect.Add("sync", sync)
		interval := feed.sync_interval / time.Second
		sect.Add("sync-interval", fmt.Sprintf("%d", int(interval)))
		sect.Add("username", feed.username)
		sect.Add("password", feed.passwd)
		sect.Add("connections", fmt.Sprintf("%d", feed.connections))
		sect = conf.NewSection(feed.Name)
		for k, v := range feed.policy.rules {
			sect.Add(k, v)
		}
	}
	return configparser.Save(conf, "feeds.ini")
}

// read config files
func ReadConfig() *SRNdConfig {

	// begin read srnd.ini

	fname := "srnd.ini"

	if os.Getenv("SRND_INI_PATH") != "" {
		if CheckFile(os.Getenv("SRND_INI_PATH")) {
			log.Printf("found SRND config at %s...", os.Getenv("SRND_INI_PATH"))
			fname = os.Getenv("SRND_INI_PATH")
		}
	}
	var s *configparser.Section
	conf, err := configparser.Read(fname)
	if err != nil {
		log.Fatal("cannot read config file ", fname)
		return nil
	}
	sconf := new(SRNdConfig)

	s, err = conf.Section("pprof")
	if err == nil {
		opts := s.Options()
		sconf.pprof = new(ProfilingConfig)
		sconf.pprof.enable = opts["enable"] == "1"
		sconf.pprof.bind = opts["bind"]
	}

	sections, _ := conf.Find("hook-*")
	if len(sections) > 0 {
		for _, hook := range sections {
			opts := hook.Options()
			sconf.hooks = append(sconf.hooks, &HookConfig{
				exec:   opts["exec"],
				enable: opts["enable"] == "1",
				name:   hook.Name(),
			})
		}
	}

	s, err = conf.Section("crypto")
	if err == nil {
		opts := s.Options()
		sconf.crypto = new(CryptoConfig)
		k := opts["tls-keyname"]
		h := opts["tls-hostname"]
		if strings.HasPrefix(h, "!") || len(h) == 0 {
			log.Fatal("please set tls-hostname to be the hostname or ip address of your server")
		} else {
			sconf.crypto.hostname = h
			sconf.crypto.privkey_file = k + "-" + h + ".key"
			sconf.crypto.cert_dir = opts["tls-trust-dir"]
			sconf.crypto.cert_file = filepath.Join(sconf.crypto.cert_dir, k+"-"+h+".crt")
		}
	} else {
		// we have no crypto section
		log.Println("!!! we will not use encryption for nntp as no crypto section is specified in srnd.ini")
	}
	s, err = conf.Section("nntp")
	if err != nil {
		log.Println("no section 'nntp' in srnd.ini")
		return nil
	}

	sconf.daemon = s.Options()

	filtersFile := s.ValueOf("filters_file")

	if filtersFile == "" {
		filtersFile = "filters.txt"
	}

	s, err = conf.Section("database")
	if err != nil {
		log.Println("no section 'database' in srnd.ini")
		return nil
	}

	sconf.database = s.Options()

	s, err = conf.Section("cache")
	if err != nil {
		log.Println("no section 'cache' in srnd.ini")
		log.Println("falling back to default cache config")
		sconf.cache = make(map[string]string)
		sconf.cache["type"] = "file"
	} else {
		sconf.cache = s.Options()
	}

	s, err = conf.Section("articles")
	if err != nil {
		log.Println("no section 'articles' in srnd.ini")
		return nil
	}

	sconf.store = s.Options()

	// frontend config

	s, err = conf.Section("frontend")

	if err != nil {
		log.Println("no frontend section in srnd.ini, disabling frontend")
		sconf.frontend = make(map[string]string)
		sconf.frontend["enable"] = "0"
	} else {
		log.Println("frontend configured in srnd.ini")
		sconf.frontend = s.Options()
		_, ok := sconf.frontend["enable"]
		if !ok {
			// default to "0"
			sconf.frontend["enable"] = "0"
		}
		enable, _ := sconf.frontend["enable"]
		if enable == "1" {
			log.Println("frontend enabled in srnd.ini")
		} else {
			log.Println("frontend not enabled in srnd.ini, disabling frontend")
		}
	}

	s, err = conf.Section("spamd")
	if err == nil {
		log.Println("spamd section found")
		sconf.spamconf.enabled = s.ValueOf("enable") == "1"
		if sconf.spamconf.enabled {
			sconf.spamconf.addr = s.ValueOf("addr")
			log.Println("spamd enabled")
		}
	}

	s, err = conf.Section("thumbnails")
	if err == nil {
		log.Println("thumbnails section found")
		sconf.thumbnails = new(ThumbnailConfig)
		sconf.thumbnails.Load(s.Options())
		sconf.thumbnails.placeholder = sconf.store["placeholder_thumbnail"]
	}

	// begin load feeds.ini

	fname = "feeds.ini"

	if os.Getenv("SRND_FEEDS_INI_PATH") != "" {
		if CheckFile(os.Getenv("SRND_FEEDS_INI_PATH")) {
			log.Printf("found feeds config at %s...", os.Getenv("SRND_FEEDS_INI_PATH"))
			fname = os.Getenv("SRND_FEEDS_INI_PATH")
		}
	}
	var confs []FeedConfig
	confs, sconf.inboundPolicy, err = feedParse(fname)
	if err != nil {
		log.Fatal("failed to load feeds: ", err)
	}

	sconf.feeds = append(sconf.feeds, confs...)

	var feeds_ok bool
	// check for feeds option
	fname, feeds_ok = sconf.daemon["feeds"]

	if feeds_ok {
		// load feeds dir first
		feeds, err := filepath.Glob(filepath.Join(fname, "*.ini"))
		if err == nil {
			for _, f := range feeds {
				log.Println("load feed", f)
				confs, _, err := feedParse(f)
				if err != nil {
					log.Fatal("failed to parse feed ", f, ": ", err)
				}
				sconf.feeds = append(sconf.feeds, confs...)
			}
		}
	}

	if CheckFile(filtersFile) {
		log.Println("loading content filter file", filtersFile)
		err = sconf.filter.LoadFile(filtersFile)
		if err != nil {
			log.Fatalf("failed to load %s: %s", filtersFile, err)
		}
		log.Printf("loaded %d filters", len(sconf.filter.globalFilters))
	}
	return sconf
}

func feedParse(fname string) (confs []FeedConfig, inboundPolicy *FeedPolicy, err error) {

	conf, err := configparser.Read(fname)

	if err != nil {
		return
	}

	default_sect, err := conf.Section("global")
	if err == nil {
		opts := default_sect.Options()
		inboundPolicy = &FeedPolicy{
			rules: opts,
		}
	}

	sections, err := conf.Find("feed-*")

	var num_sections int
	num_sections = len(sections)

	if num_sections > 0 {
		// load feeds
		for _, sect := range sections {
			var fconf FeedConfig
			// check for proxy settings
			val := sect.ValueOf("proxy-type")
			if len(val) > 0 && strings.ToLower(val) != "none" {
				fconf.proxy_type = strings.ToLower(val)
				proxy_host := sect.ValueOf("proxy-host")
				proxy_port := sect.ValueOf("proxy-port")
				fconf.proxy_addr = strings.Trim(proxy_host, " ") + ":" + strings.Trim(proxy_port, " ")
			}

			host := sect.ValueOf("host")
			port := sect.ValueOf("port")

			// check to see if we want to sync with them first
			val = sect.ValueOf("sync")
			if val == "1" {
				fconf.sync = true
				// sync interval in seconds
				i := mapGetInt(sect.Options(), "sync-interval", 60)
				if i < 60 {
					i = 60
				}
				fconf.sync_interval = time.Second * time.Duration(i)
			}

			// check for feed disabled
			fconf.disable = sect.ValueOf("disable") == "1"

			// concurrent connection count
			fconf.connections = mapGetInt(sect.Options(), "connections", 1)

			// username / password auth
			fconf.username = sect.ValueOf("username")
			fconf.passwd = sect.ValueOf("password")
			fconf.tls_off = sect.ValueOf("disabletls") == "1"

			// load feed polcies
			sect_name := sect.Name()[5:]
			fconf.Name = sect_name
			if len(host) > 0 && len(port) > 0 {
				// host port specified
				fconf.Addr = host + ":" + port
			} else {
				// no host / port specified
				fconf.Addr = strings.Trim(sect_name, " ")
			}
			feed_sect, err := conf.Section(sect_name)
			if err != nil {
				log.Fatal("no section", sect_name, "in ", fname)
			}
			opts := feed_sect.Options()
			fconf.policy.rules = make(map[string]string)
			for k, v := range opts {
				fconf.policy.rules[k] = v
			}
			confs = append(confs, fconf)
		}
	}
	return
}

// fatals on failed validation
func (self *SRNdConfig) Validate() {
	// check for daemon section entries
	daemon_param := []string{"bind", "instance_name", "allow_anon", "allow_anon_attachments"}
	for _, p := range daemon_param {
		_, ok := self.daemon[p]
		if !ok {
			log.Fatalf("in section [nntp], no parameter '%s' provided", p)
		}
	}

	// check validity of store directories
	store_dirs := []string{"store", "incoming", "attachments", "thumbs"}
	for _, d := range store_dirs {
		k := d + "_dir"
		_, ok := self.store[k]
		if !ok {
			log.Fatalf("in section [store], no parameter '%s' provided", k)
		}
	}

	// check database parameters existing
	db_param := []string{"host", "port", "user", "password", "type", "schema"}
	for _, p := range db_param {
		_, ok := self.database[p]
		if !ok {
			log.Fatalf("in section [database], no parameter '%s' provided", p)
		}
	}
}
