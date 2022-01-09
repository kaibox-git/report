package report

import (
	"errors"
	"fmt"
	"net/mail"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/kaibox-git/lmail"
	"github.com/kaibox-git/sqlparams"
)

var (
	QueryTimeWarning = 200 * time.Millisecond // Высылает предупреждение, если время выполнения запроса превышает установленное
	QueryDeadline    = 5 * time.Second        // Прерывает запрос, освобождая соединение, если время выполнения запроса превышает установленное
	ErrNotFound      = errors.New("not found")
	ErrContext       = errors.New(`context canceled or deadline exceeded`)
	ErrInternal      = errors.New("internal server error")
)

type Request struct {
	Data string
}

type Report struct {
	appName string
	email   emailConfig
}

type emailConfig struct {
	sender lmail.EmailProvider
	from   mail.Address
	to     []mail.Address
}

func New(appName string, mailSender lmail.EmailProvider, from mail.Address, to []mail.Address) *Report {
	return &Report{
		appName: appName,
		email: emailConfig{
			sender: mailSender,
			from:   from,
			to:     to,
		},
	}
}

func (report *Report) Sql(query string, params ...interface{}) {
	var FileWithLineNum = FileWithLineNum()
	fmt.Printf("\n%s\n%s\n", FileWithLineNum, sqlparams.Inline(query, params...))
}

// Сообщение об ошибке выполнения sql запроса
func (report *Report) SqlError(r *Request, err error, query string, params ...interface{}) {
	var FileWithLineNum = FileWithLineNum()
	if err == nil {
		fmt.Printf("\n%s\n%s\n", FileWithLineNum, sqlparams.Inline(query, params...))
		return
	}
	fmt.Printf("\n%s:\n%s\n%s\n", FileWithLineNum, err.Error(), sqlparams.Inline(query, params...))

	m := report.createMessage(r, FileWithLineNum, err, sqlparams.Inline(query, params...))
	go func() {
		_ = report.email.sender.Send(&lmail.Data{
			From:        report.email.from,
			To:          report.email.to,
			Subject:     `!!! SQL проблема !!!`,
			Body:        m,
			WithLimiter: true,
		})
	}()
}

func (report *Report) Error(r *Request, err error) {
	var FileWithLineNum = FileWithLineNum()
	fmt.Printf("\n%s\n%s\n", FileWithLineNum, err.Error())

	m := report.createMessage(r, FileWithLineNum, err, "")
	go func() {
		_ = report.email.sender.Send(&lmail.Data{
			From:        report.email.from,
			To:          report.email.to,
			Subject:     `!!! Ошибка !!!`,
			Body:        m,
			WithLimiter: true,
		})
	}()
}

func (report *Report) createMessage(r *Request, FileWithLineNum string, err error, sql string) string {
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
			requestData(r))
	} else {
		fmt.Fprintf(&sb, "%s\n%s\n%s\n%s",
			time.Now().Format("02.01.2006 15:04:05"),
			FileWithLineNum,
			errStr,
			requestData(r))
	}
	return sb.String()
}

func FileWithLineNum() string {
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

func requestData(r *Request) string {
	return r.Data
}
