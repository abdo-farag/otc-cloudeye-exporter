package logs

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"gopkg.in/yaml.v3"
)

// LoggerConstructor wraps the main logger instance.
type LoggerConstructor struct {
	LogInstance logger
}

// logger interface defines the expected log methods.
type logger interface {
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
	Fatal(args ...interface{})
	Debugf(template string, args ...interface{})
	Infof(template string, args ...interface{})
	Warnf(template string, args ...interface{})
	Errorf(template string, args ...interface{})
	Fatalf(template string, args ...interface{})
	Sync() error
}

// Config defines a single logging target and its parameters.
type Config struct {
	Level      LogLevel `yaml:"level"`
	Type       string        `yaml:"type"` // "FILE" or "STDOUT"
	Filename   string        `yaml:"filename,omitempty"`
	Encoder    string        `yaml:"encoder,omitempty"` // "JSON" or "CONSOLE"
	TimeFormat string        `yaml:"time_format,omitempty"`
	MaxSize    int           `yaml:"max_size,omitempty"` // bytes
	MaxBackups int           `yaml:"max_backups,omitempty"`
	MaxAge     int           `yaml:"max_age,omitempty"` // days
	Enabled    bool          `yaml:"enabled"`
	Compress   bool          `yaml:"compress"`
}

// ----- Global Logger instance -----
var Logger LoggerConstructor

// ---- Exposed convenience functions ----
func Debug(args ...interface{})                { Logger.Debug(args...) }
func Debugf(format string, args ...interface{}) { Logger.Debugf(format, args...) }
func Info(args ...interface{})                 { Logger.Info(args...) }
func Infof(format string, args ...interface{})  { Logger.Infof(format, args...) }
func Warn(args ...interface{})                 { Logger.Warn(args...) }
func Warnf(format string, args ...interface{})  { Logger.Warnf(format, args...) }
func Error(args ...interface{})                { Logger.Error(args...) }
func Errorf(format string, args ...interface{}) { Logger.Errorf(format, args...) }
func Fatal(args ...interface{})                { Logger.Fatal(args...) }
func Fatalf(format string, args ...interface{}) { Logger.Fatalf(format, args...) }
func Flush()                                   { Logger.Flush() }

type LogLevel zapcore.Level

func (l *LogLevel) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var levelStr string
	if err := unmarshal(&levelStr); err != nil {
		return err
	}
	level := zapcore.InfoLevel // default
	switch strings.ToLower(levelStr) {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	case "dpanic":
		level = zapcore.DPanicLevel
	case "panic":
		level = zapcore.PanicLevel
	case "fatal":
		level = zapcore.FatalLevel
	default:
		return fmt.Errorf("unknown log level: %q", levelStr)
	}
	*l = LogLevel(level)
	return nil
}


// ---- Configuration loader ----

type Unmarshal func(data []byte, cfg interface{}) error

type ConfLoader struct {
	Unmarshal Unmarshal
}

func newYamlLoader() *ConfLoader {
	return newConfLoader(yaml.Unmarshal)
}

func newConfLoader(u Unmarshal) *ConfLoader {
	return &ConfLoader{
		Unmarshal: u,
	}
}

func (c ConfLoader) LoadFile(fPath string, cfg interface{}) error {
	data, err := ioutil.ReadFile(fPath)
	if err != nil {
		return err
	}
	return c.LoadData(data, cfg)
}

func (c ConfLoader) LoadData(data []byte, cfg interface{}) error {
	return c.Unmarshal(data, cfg)
}

// ---- Logger Initialization ----

// InitLog initializes the logger from a YAML config file using the "logging" key.
func InitLog(logsConfPath string) {
	realPath, pathErr := NormalizePath(logsConfPath)
	if pathErr != nil {
		fmt.Printf("Normalize log config path error: %s\n", pathErr.Error())
		return
	}
	var cfg map[string][]Config
	err := newYamlLoader().LoadFile(realPath, &cfg)
	if err != nil {
		fmt.Printf("Fail to load logs.yml, error: %s\n", err.Error())
		return
	}
	config, ok := cfg["logging"]
	if !ok {
		fmt.Printf("logs.yml should contain a 'logging' config.\n")
		return
	}
	Logger.LogInstance = makeZapLogger(config).WithOptions(zap.AddCallerSkip(1)).Sugar()
}

// ---- Logger Methods ----

func (zap *LoggerConstructor) Debug(args ...interface{})  { zap.LogInstance.Debug(clearLineBreaks("", args...)) }
func (zap *LoggerConstructor) Info(args ...interface{})   { zap.LogInstance.Info(clearLineBreaks("", args...)) }
func (zap *LoggerConstructor) Warn(args ...interface{})   { zap.LogInstance.Warn(clearLineBreaks("", args...)) }
func (zap *LoggerConstructor) Error(args ...interface{})  { zap.LogInstance.Error(clearLineBreaks("", args...)) }
func (zap *LoggerConstructor) Fatal(args ...interface{})  { zap.LogInstance.Fatal(clearLineBreaks("", args...)) }
func (zap *LoggerConstructor) Debugf(template string, args ...interface{}) { zap.LogInstance.Debugf(clearLineBreaks(template, args...)) }
func (zap *LoggerConstructor) Infof(template string, args ...interface{})  { zap.LogInstance.Infof(clearLineBreaks(template, args...)) }
func (zap *LoggerConstructor) Warnf(template string, args ...interface{})  { zap.LogInstance.Warnf(clearLineBreaks(template, args...)) }
func (zap *LoggerConstructor) Errorf(template string, args ...interface{}) { zap.LogInstance.Errorf(clearLineBreaks(template, args...)) }
func (zap *LoggerConstructor) Fatalf(template string, args ...interface{}) { zap.LogInstance.Fatalf(clearLineBreaks(template, args...)) }
func (zap *LoggerConstructor) Flush() {
	if err := zap.LogInstance.Sync(); err != nil {
		fmt.Printf("Fail to sync logs, error: %s\n", err.Error())
	}
}

// For graceful shutdown:
func FlushLogAndExit(code int) {
	Flush()
	os.Exit(code)
}

// ---- Utilities ----

// getMessage returns a formatted log message
func getMessage(template string, fmtArgs []interface{}) string {
	if len(fmtArgs) == 0 {
		return template
	}
	if template != "" {
		return fmt.Sprintf(template, fmtArgs...)
	}
	if len(fmtArgs) == 1 {
		if str, ok := fmtArgs[0].(string); ok {
			return str
		}
	}
	return fmt.Sprint(fmtArgs...)
}

// clearLineBreaks strips unsafe characters from log messages
func clearLineBreaks(template string, args ...interface{}) string {
	message := getMessage(template, args)
	if message != "" {
		// Prevent log injection by removing control characters
		for _, ch := range []string{"\b", "\n", "\t", "\u000b", "\f", "\r", "\u007f"} {
			message = strings.ReplaceAll(message, ch, "")
		}
	}
	return message
}

// ---- Zap core/encoder/rotation ----

func makeRotate(file string, maxSize int, maxBackups int, maxAge int, compress bool) *lumberjack.Logger {
	return &lumberjack.Logger{
		Filename:   file,
		MaxSize:    maxSize / 1024 / 1024, // Convert bytes to megabytes
		MaxBackups: maxBackups,
		MaxAge:     maxAge,
		LocalTime:  true,
		Compress:   compress,
	}
}

func makeEncoder(c *Config) zapcore.Encoder {
	encoderConfig := zap.NewProductionEncoderConfig()
	if c.TimeFormat == "" {
		c.TimeFormat = "02.01.2006 15:04:05"
	}
	encoderConfig.EncodeTime = func(ts time.Time, encoder zapcore.PrimitiveArrayEncoder) {
		encoder.AppendString(ts.Format(c.TimeFormat))
	}
	encoderConfig.EncodeDuration = func(d time.Duration, encoder zapcore.PrimitiveArrayEncoder) {
		val := float64(d) / float64(time.Millisecond)
		encoder.AppendString(fmt.Sprintf("%.3fms", val))
	}
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	if strings.ToUpper(c.Encoder) == "JSON" {
		return zapcore.NewJSONEncoder(encoderConfig)
	}
	return zapcore.NewConsoleEncoder(encoderConfig)
}

func makeWriteSync(c *Config) zapcore.WriteSyncer {
	if strings.ToUpper(c.Type) == "FILE" {
		logRotate := makeRotate(c.Filename, c.MaxSize, c.MaxBackups, c.MaxAge, c.Compress)
		return zapcore.AddSync(logRotate)
	}
	return zapcore.AddSync(os.Stdout)
}

func makeZapCore(c *Config) zapcore.Core {
	encoder := makeEncoder(c)
	w := makeWriteSync(c)
	core := zapcore.NewCore(encoder, w, zapcore.Level(c.Level))
	return core
}

func makeZapLogger(cfg []Config) *zap.Logger {
	cores := make([]zapcore.Core, 0, len(cfg))
	for i := range cfg {
		if !cfg[i].Enabled {
			continue
		}
		cores = append(cores, makeZapCore(&cfg[i]))
	}
	return zap.New(zapcore.NewTee(cores...), zap.AddCaller())
}

// ---- Path Normalization ----

func NormalizePath(path string) (string, error) {
	relPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	relPath = strings.ReplaceAll(relPath, "\\", "/")
	match, err := regexp.MatchString(`[!;<>&|$\n`+"`"+`\\]`, relPath)
	if match || err != nil {
		return "", errors.New("invalid characters in path")
	}
	return relPath, nil
}
