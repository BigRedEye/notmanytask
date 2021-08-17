package conf

import (
	"flag"
	"reflect"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

var configPath string

func init() {
	flag.StringVar(&configPath, "config", "", "Path to the config")
	flag.Parse()
}

type Option interface {
	apply(v *viper.Viper)
}

type envPrefix struct {
	prefix string
}

func (p *envPrefix) apply(v *viper.Viper) {
	v.SetEnvPrefix(p.prefix)
}

func EnvPrefix(prefix string) Option {
	return &envPrefix{prefix}
}

// https://github.com/spf13/viper/issues/188#issuecomment-399884438
func bindEnvs(iface interface{}, parts ...string) {
	ifv := reflect.ValueOf(iface)
	ift := reflect.TypeOf(iface)

	if ifv.Kind() == reflect.Ptr {
		bindEnvs(ifv.Elem().Interface(), parts...)
		return
	}

	for i := 0; i < ift.NumField(); i++ {
		v := ifv.Field(i)
		t := ift.Field(i)
		name, ok := t.Tag.Lookup("mapstructure")
		if !ok {
			name = t.Name
		}
		if v.Kind() == reflect.Struct {
			bindEnvs(v.Interface(), append(parts, name)...)
		} else {
			err := viper.BindEnv(strings.Join(append(parts, name), "."))
			if err != nil {
				panic(err)
			}
		}
	}
}

func ParseConfig(config interface{}, options ...Option) error {
	for _, option := range options {
		option.apply(viper.GetViper())
	}
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if len(configPath) > 0 {
		viper.SetConfigFile(configPath)
		if err := viper.ReadInConfig(); err != nil {
			return errors.Wrap(err, "Failed to load config")
		}
	}

	bindEnvs(config)

	if err := viper.Unmarshal(config); err != nil {
		return errors.Wrap(err, "Failed to unmarshal config")
	}

	return nil
}
