package log

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var logger *zap.Logger

func init() {
	writer := &lumberjack.Logger{
		// TODO Set in the configuration file
		Filename:   "logs/run.log",
		MaxSize:    30, // MB
		MaxBackups: 10,
		MaxAge:     7, // Days
		Compress:   false,
	}

	core := zapcore.NewCore(
		newEncoder(),
		//zapcore.AddSync(os.Stdout),
		zapcore.AddSync(writer),
		zapcore.InfoLevel,
	)
	logger = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel))
}

func newEncoder() zapcore.Encoder {
	return zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
		TimeKey:  "at",
		LevelKey: "level",
		NameKey:  "logger",
		//CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		FunctionKey:    zapcore.OmitKey,
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	})
}

func GetLogger() *zap.Logger {
	return logger
}
