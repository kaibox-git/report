# Report

The report package contains methods for bots to notify a developer about runtime errors.

## Install

```
go get github.com/kaibox-git/report
```

## Usage

```go
...

// init [email client](https://github.com/kaibox-git/lmail)
host := `localhost`
port := 25
connTimeout := time.Second // for local smtp server
/*
Only 20 emails per 30 minutes. The rest is ignored.
This is useful for notifications of errors, but has a limitation if emailing is too often.
In this case keep logging info to file.
*/
emailNumber := 20 
period := 30 * time.Minute
emailSender, err := smtpclient.New(host, port, connTimeout, emailNumber, period)
if err != nil {
    println(err.Error())
    os.Exit(1)
}

// init simple logger for demonstration purposes
errorLogger := log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)

// init report client
appName := `mybot`
from := mail.Address{`Robot`,`no-reply@domain.com`}
to := []mail.Address{
    {`To me`,`developer@domain.com`},
}

report, err := report.New(appName, emailSender, from, to, errorLogger)
if err != nil {
    println(err.Error())
    os.Exit(1)
}

// Now we are ready to use the report

...

url := `https://domain.com`
request, err := http.NewRequest("GET", url, nil)
if err != nil {
    report.Error(url, err) // you can pass 'url' as first parameter to include this value to report indicating that the error belongs to this value
    return report.ErrInternal
}

...

func (repo *SomeRepo) SelectSomeTable(ctx context.Context, SubjectId int) (uint, error) {
	// Timings (QueryDeadline & QueryTimeWarning):
	queryCtx, cancel := context.WithTimeout(ctx, repo.QueryDeadline) // Cancel query if QueryDeadline is exceeded.
	defer cancel()
	queryStart := time.Now()

	var id uint
    // for postgres:
	query := repo.db.Rebind(`SELECT id FROM some_table WHERE subject_id=? AND actual`)
	params := []interface{}{SubjectId}
	if err := repo.db.QueryRowContext(queryCtx, query, params...).Scan(&id); err != nil {
		switch {
		case errors.Is(queryCtx.Err(), context.Canceled) || errors.Is(queryCtx.Err(), context.DeadlineExceeded):
			return 0, uerror.Context
		case err == sql.ErrNoRows:
			return 0, uerror.NotFound
		default:
            // If there is some context data (object) to this query that you want to see in report you can pass it as first parameter.
            // Now this is nil.
			repo.report.SqlError(nil, err, query, params...) 
			return 0, report.ErrReported
		}
	}

	// Warning if QueryTimeWarning is exceeded.
	queryTime := time.Since(queryStart)
	if queryTime > repo.QueryTimeWarning {
		repo.report.SqlError(nil, fmt.Errorf("query time: %v", queryTime), query, params...)
	}

	return id, nil
}

```

While testing you can use report.Sql()

```go
subjectId = 5
categoryId = 2
query := `SELECT id FROM some_table WHERE subject_id=? AND category_id=?`
params := []interface{}{subjectId, categoryId}
report.Sql(query, params...)
```
to print sql query with inlined parameters to stdout (console):
```sql
SELECT id FROM some_table WHERE subject_id=5 AND category_id=2
```
Now you can execute this query in the database console.