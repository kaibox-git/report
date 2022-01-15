package report

import (
	"errors"
	"fmt"
	"log"
	"net/mail"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kaibox-git/lmail"
	"github.com/kaibox-git/sqlparams"
)

var (
	QueryTimeWarning = 200 * time.Millisecond // Высылает предупреждение, если время выполнения запроса превышает установленное
	QueryDeadline    = 5 * time.Second        // Прерывает запрос, освобождая соединение, если время выполнения запроса превышает установленное
	ErrReported      = errors.New(`error has been reported`)
	ErrNotFound      = errors.New("not found")
	ErrContext       = errors.New(`context canceled`)
	ErrInternal      = errors.New("internal server error")
)

type ReportProvider interface {
	Message(m string)
	Sql(query string, params ...interface{})
	SqlError(r interface{}, err error, query string, params ...interface{})
	Error(r interface{}, err error)
	FileWithLineNum() string
	FilesWithLineNum() (out []string)
}

type Report struct {
	appName  string
	email    emailConfig
	errorLog *log.Logger
}

type emailConfig struct {
	sender lmail.EmailProvider
	from   mail.Address
	to     []mail.Address
}

func New(appName string, mailSender lmail.EmailProvider, from mail.Address, to []mail.Address, errorLogger *log.Logger) *Report {
	return &Report{
		appName: appName,
		email: emailConfig{
			sender: mailSender,
			from:   from,
			to:     to,
		},
		errorLog: errorLogger,
	}
}

func (report *Report) Message(subject, body string) {
	if body == `` {
		body = subject
	}
	m := fmt.Sprintf("%s\n%s\n\n", time.Now().Format("02.01.2006 15:04:05"), body)
	print(m)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := report.email.sender.Send(&lmail.Data{
			From:    report.email.from,
			To:      report.email.to,
			Subject: subject,
			Body:    m,
		}); err != nil {
			report.logError(m)
		}
	}()
	if report.errorLog != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			report.logError(m)
		}()
	}
	wg.Wait()
}

// Для теста. Выводит в stdout sql запрос с встроенными параметрами
func (report *Report) Sql(query string, params ...interface{}) {
	var FileWithLineNum = report.FileWithLineNum()
	fmt.Printf("\n%s\n%s\n", FileWithLineNum, sqlparams.Inline(query, params...))
}

// Сообщение об ошибке выполнения sql запроса
func (report *Report) SqlError(r interface{}, err error, query string, params ...interface{}) {
	var FileWithLineNum = report.FileWithLineNum()
	if err == nil {
		fmt.Printf("\n%s\n%s\n", FileWithLineNum, sqlparams.Inline(query, params...))
		return
	}
	fmt.Printf("\n%s:\n%s\n%s\n", FileWithLineNum, err.Error(), sqlparams.Inline(query, params...))

	m := report.createMessage(r, FileWithLineNum, err, sqlparams.Inline(query, params...))
	go func() {
		if err := report.email.sender.Send(&lmail.Data{
			From:        report.email.from,
			To:          report.email.to,
			Subject:     `!!! SQL проблема !!!`,
			Body:        m,
			WithLimiter: true,
		}); err != nil {
			if report.errorLog != nil {
				report.logError(m)
			}
		}
	}()
	if report.errorLog != nil {
		go report.logError(m)
	}

}

func (report *Report) Error(r interface{}, err error) {
	var FileWithLineNum = report.FileWithLineNum()
	fmt.Printf("\n%s\n%s\n", FileWithLineNum, err.Error())

	m := report.createMessage(r, FileWithLineNum, err, "")
	go func() {
		if err := report.email.sender.Send(&lmail.Data{
			From:        report.email.from,
			To:          report.email.to,
			Subject:     `!!! Ошибка !!!`,
			Body:        m,
			WithLimiter: true,
		}); err != nil {
			if report.errorLog != nil {
				report.logError(m)
			}
		}
	}()
	if report.errorLog != nil {
		go report.logError(m)
	}
}

func (report *Report) createMessage(r interface{}, FileWithLineNum string, err error, sql string) string {
	var sb strings.Builder
	var errStr string
	if err != nil {
		errStr = err.Error() + "\n"
	} else {
		errStr = "\n"
	}
	if len(sql) > 0 {
		fmt.Fprintf(&sb, "%s\n%s\n%s\n%s\n\n%s",
			time.Now().Format("02.01.2006 15:04:05"),
			FileWithLineNum,
			errStr,
			sql,
			getObjectData(r))
	} else {
		fmt.Fprintf(&sb, "%s\n%s\n%s\n%s",
			time.Now().Format("02.01.2006 15:04:05"),
			FileWithLineNum,
			errStr,
			getObjectData(r))
	}
	return sb.String()
}

func (report *Report) FileWithLineNum() string {
	for i := 2; i < 15; i++ {
		_, file, line, ok := runtime.Caller(i)
		if ok {
			return file + ":" + strconv.FormatInt(int64(line), 10)
		}
	}
	return ``
}

func (report *Report) FilesWithLineNum() (out []string) {
	for i := 0; i < 15; i++ {
		_, file, line, ok := runtime.Caller(i)
		if ok && strings.Contains(file, `/`+report.appName+`/`) {
			out = append(out, file+":"+strconv.FormatInt(int64(line), 10))
		}
	}
	return out
}

func (report *Report) logError(m string) {
	report.errorLog.Printf("%s", m)
	report.errorLog.Println(strings.Repeat("—", 70))
	report.errorLog.Println("")
}

func getObjectData(object interface{}, tabs ...string) string {
	if object == nil {
		return ``
	}
	t := reflect.TypeOf(object)
	val := reflect.ValueOf(object)
	if t.Kind() == reflect.Ptr {
		val = val.Elem()
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return "\n"
	}
	var (
		key          string
		v            interface{}
		sb           strings.Builder
		timeType     = reflect.TypeOf(time.Time{})
		tab          = strings.Join(tabs, ``)
		handleStruct = func(i int) {
			switch {
			case t.Field(i).Type == timeType: // time.Time
				fmt.Fprintf(&sb, "%s%s: %v\n", tab, t.Field(i).Name, val.Field(i))
			case t.Field(i).Type.Kind() == reflect.Ptr && t.Field(i).Type.Elem() == timeType: // *time.Time
				fmt.Fprintf(&sb, "%s%s: %v\n", tab, t.Field(i).Name, val.Field(i).Elem())
			default: // struct or *struct
				fmt.Fprintf(&sb, "%s%s:\n", tab, t.Field(i).Name)
				sb.WriteString(getObjectData(val.Field(i).Interface(), tab, "\t"))
			}
		}
	)
	for i := 0; i < val.NumField(); i++ {
		if !val.Field(i).CanInterface() { // Если поле неэкспортируемое, - проигнорировать
			continue
		}
		switch t.Field(i).Type.Kind() {
		case reflect.Ptr:
			if val.Field(i).IsNil() {
				v = nil
			} else if t.Field(i).Type.Elem().Kind() == reflect.Struct {
				handleStruct(i)
				continue
			} else {
				v = val.Field(i).Elem().Interface()
			}
			key = t.Field(i).Name
		case reflect.Struct:
			handleStruct(i)
			continue
		default:
			key = t.Field(i).Name
			v = val.Field(i).Interface()
		}
		fmt.Fprintf(&sb, "%s%s = %v\n", tab, key, v)
	}
	sb.WriteByte('\n')
	return sb.String()

}
