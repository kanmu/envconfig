# envconfig

This is a fork of https://github.com/kelseyhightower/envconfig.  Kudos to the original authors for saving me a lot of time!"


[![Build Status](https://travis-ci.org/moriyoshi/envconfig.svg)](https://travis-ci.org/moriyoshi/envconfig)

```Go
import "github.com/moriyoshi/envconfig"
```

## Documentation

See [godoc](http://godoc.org/github.com/moriyoshi/envconfig)

## Usage

Set some environment variables:

```Bash
export MYAPP_DEBUG=false
export MYAPP_PORT=8080
export MYAPP_USER=Kelsey
export MYAPP_RATE="0.5"
export MYAPP_TIMEOUT="3m"
export MYAPP_USERS="rob,ken,robert"
export MYAPP_COLORCODES="red:1,green:2,blue:3"
```

Write some code:

```Go
package main

import (
    "fmt"
    "log"
    "time"

    "github.com/kelseyhightower/envconfig"
)

type Specification struct {
    Debug       bool
    Port        int
    User        string
    Users       []string
    Rate        float32
    Timeout     time.Duration
    ColorCodes  map[string]int
}

func main() {
    var s Specification
    err := envconfig.Process("myapp", &s)
    if err != nil {
        log.Fatal(err.Error())
    }
    format := "Debug: %v\nPort: %d\nUser: %s\nRate: %f\nTimeout: %s\n"
    _, err = fmt.Printf(format, s.Debug, s.Port, s.User, s.Rate, s.Timeout)
    if err != nil {
        log.Fatal(err.Error())
    }

    fmt.Println("Users:")
    for _, u := range s.Users {
        fmt.Printf("  %s\n", u)
    }

    fmt.Println("Color codes:")
    for k, v := range s.ColorCodes {
        fmt.Printf("  %s: %d\n", k, v)
    }
}
```

Results:

```Bash
Debug: false
Port: 8080
User: Kelsey
Rate: 0.500000
Timeout: 3m0s
Users:
  rob
  ken
  robert
Color codes:
  red: 1
  green: 2
  blue: 3
```

## Struct Tag Support

Envconfig supports the use of struct tags to specify alternate, default, and required
environment variables.

For example, consider the following struct:

```Go
type Specification struct {
    ManualOverride1 string `envconfig:"manual_override_1"`
    DefaultVar      string `envconfig_default:"foobar"`
    RequiredVar     string `envconfig_required:"true"`
    IgnoredVar      string `envconfig_ignored:"true"`
    AutoSplitVar    string `envconfig_snake:"true"`
}
```

Envconfig has automatic support for CamelCased struct elements when the
`envconfig_snake:"true"` tag is supplied. Without this tag, `AutoSplitVar` above
would look for an environment variable called `MYAPP_AUTOSPLITVAR`. With the
setting applied it will look for `MYAPP_AUTO_SPLIT_VAR`. Note that numbers
will get globbed into the previous word. If the setting does not do the
right thing, you may use a manual override.

Envconfig will process value for `ManualOverride1` by populating it with the
value for `MYAPP_MANUAL_OVERRIDE_1`. Without this struct tag, it would have
instead looked up `MYAPP_MANUALOVERRIDE1`. With the `envconfig_snake:"true"` tag
it would have looked up `MYAPP_MANUAL_OVERRIDE1`.

```Bash
export MYAPP_MANUAL_OVERRIDE_1="this will be the value"

# export MYAPP_MANUALOVERRIDE1="and this will not"
```

If envconfig can't find an environment variable value for `MYAPP_DEFAULTVAR`,
it will populate it with "foobar" as a default value.

If envconfig can't find an environment variable value for `MYAPP_REQUIREDVAR`,
it will return an error when asked to process the struct.

If envconfig can't find an environment variable in the form `PREFIX_MYVAR`, and there
is a struct tag defined, it will try to populate your variable with an environment
variable that directly matches the envconfig tag in your struct definition unless
`envconfig_alt_prefixed:"false"` is specified:

```shell
export SERVICE_HOST=127.0.0.1
export MYAPP_DEBUG=true
```
```Go
type Specification struct {
    ServiceHost string `envconfig:"SERVICE_HOST"`
    Debug       bool
}
```

Otherwise, the environment variable that exactly matches the value of `envconfig`
tag will be looked up:

```Go
type Specification struct {
    ServiceHost string `envconfig:"SERVICE_HOST" envconfig_alt_prefixed:"false"`
    Debug       bool
}
```

You can specify multiple aliases to `envconfig` tag by delimiting each name by equal sign (`=`).

```Go
type Specification struct {
    ServiceHost string `envconfig:"SERVICE_HOST=SERVICEHOST"`
    Debug       bool
}
```

Envconfig won't process a field with the `envconfig_ignored` tag set to `true`, even if a corresponding
environment variable is set.

## Supported Struct Field Types

envconfig supports these struct field types:

  * string
  * int8, int16, int32, int64
  * bool
  * float32, float64
  * slices of any supported type
  * maps (keys and values of any supported type)
  * [encoding.TextUnmarshaler](https://golang.org/pkg/encoding/#TextUnmarshaler)
  * [encoding.BinaryUnmarshaler](https://golang.org/pkg/encoding/#BinaryUnmarshaler)

Embedded structs using these fields are also supported.

## Custom Decoders

### envconfig.Decoder

Any field whose type (or pointer-to-type) implements `envconfig.Decoder` can
control its own deserialization:

```Bash
export DNS_SERVER=8.8.8.8
```

```Go
type IPDecoder net.IP

func (ipd *IPDecoder) Decode(value string) error {
    *ipd = IPDecoder(net.ParseIP(value))
    return nil
}

type DNSConfig struct {
    Address IPDecoder `envconfig:"DNS_SERVER"`
}
```

Also, envconfig will use a `Set(string) error` method like from the
[flag.Value](https://godoc.org/flag#Value) interface if implemented.

### envconfig_setter tag

Or you can apply custom decoder to a field by supplying `envconfig_setter` tag.

```Go
type Specification struct {
    Foo string `envconfig_setter:"DecodeFoo"`
}

func (*Specification) DecodeFoo(s *string, value string) error {
    s = value + "!"
    return nil
}

```
