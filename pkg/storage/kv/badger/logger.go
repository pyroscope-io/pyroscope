package badger

import "github.com/sirupsen/logrus"

type Logger struct {
	name     string
	logLevel logrus.Level
}

func (b Logger) Errorf(firstArg string, args ...interface{}) {
	if b.logLevel < logrus.ErrorLevel {
		return
	}
	logrus.WithField("badger", b.name).Errorf(firstArg, args...)
}

func (b Logger) Warningf(firstArg string, args ...interface{}) {
	if b.logLevel < logrus.WarnLevel {
		return
	}
	logrus.WithField("badger", b.name).Warningf(firstArg, args...)
}

func (b Logger) Infof(firstArg string, args ...interface{}) {
	if b.logLevel < logrus.InfoLevel {
		return
	}
	logrus.WithField("badger", b.name).Infof(firstArg, args...)
}

func (b Logger) Debugf(firstArg string, args ...interface{}) {
	if b.logLevel < logrus.DebugLevel {
		return
	}
	logrus.WithField("badger", b.name).Debugf(firstArg, args...)
}
