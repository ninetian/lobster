package lobster

import "github.com/LunaNode/email"

import "bytes"
import "errors"
import "fmt"
import "io/ioutil"
import "log"
import "strings"
import "text/template"

type EmailParams struct {
	Username string
	Email string
	Params interface{}
}

type ErrorEmail struct {
	Error string
	Description string
	Detail string
}

type LowCreditEmail struct {
	Credit int64
	Hourly int64
	RemainingHours int
}

type VmUnsuspendEmail struct {
	Name string
}

type VmDeletedEmail struct {
	Id int
	Name string
}

type VmCreateEmail struct {
	Id int
	Name string
}

type VmCreateErrorEmail struct {
	Id int
	Name string
}

type TicketUpdateEmail struct {
	Id int
	Subject string
	Message string
}

type CoinbaseMispaidEmail struct {
	OrderId string
}

type PaymentProcessedEmail *Transaction

type AccountCreatedEmail struct {
	UserId int
	Username string
	Email string
}

type BandwidthUsageEmail struct {
	UtilPercent int
	Region string
	Fee int64
}

var emailTemplate *template.Template

func loadEmail() {
	contents, _ := ioutil.ReadDir("tmpl/email")
	templatePaths := make([]string, 0)
	for _, fileInfo := range contents {
		if fileInfo.Mode().IsRegular() && strings.HasSuffix(fileInfo.Name(), ".txt") {
			templatePaths = append(templatePaths, "tmpl/email/" + fileInfo.Name())
		}
	}
	emailTemplate = template.Must(template.New("").Funcs(template.FuncMap(templateFuncMap())).ParseFiles(templatePaths...))
}

func reportError(err error, description string, detail string) {
	if err != nil {
		if detail != "" {
			log.Println(detail)
		}
		log.Printf(fmt.Sprintf("%s: error: %s", description, err))

		// the mail operation may itself generate an error, but we don't recursively report it
		suberr := mail(nil, -1, "error", ErrorEmail{Error: err.Error(), Description: description, Detail: detail}, false)
		if suberr != nil {
			log.Printf("reportError: failed to report: %s", suberr)
		}
	}
}

func mail(db *Database, userId int, tmpl string, subparams interface{}, ccAdmin bool) error {
	toAddress := cfg.Default.AdminEmail
	username := "N/A"

	if userId >= 0 && db != nil {
		user := userDetails(db, userId)
		if user == nil {
			return errors.New("user does not exist")
		}

		if user.Status != "new" {
			toAddress = user.Email
		} else {
			toAddress = ""
		}
		username = user.Username

		if toAddress == "" && !ccAdmin {
			return nil
		}
	}

	params := EmailParams{
		Username: username,
		Email: toAddress,
		Params: subparams,
	}

	var buffer bytes.Buffer
	err := emailTemplate.ExecuteTemplate(&buffer, tmpl + ".txt", params)
	if err != nil {
		return err
	}
	templateParts := strings.SplitN(buffer.String(), "\n\n", 2)
	if len(templateParts) != 2 {
		return errors.New("template output does not include subject/body separator")
	}

	e := email.NewEmail()
	e.From = cfg.Default.FromEmail
	if toAddress != "" {
		e.To = []string{toAddress}
	}
	if ccAdmin {
		e.Bcc = []string{cfg.Default.AdminEmail}
	}
	e.Subject = templateParts[0]
	e.Text = []byte(templateParts[1])
	log.Printf("Sending email [%s] to [%s]", e.Subject, toAddress)
	return e.Send(cfg.Email.Host, cfg.Email.Port, nil, cfg.Email.NoTLS)
}

func mailWrap(db *Database, userId int, tmpl string, subparams interface{}, ccAdmin bool) {
	go func() {
		defer errorHandler(nil, nil, true)
		err := mail(db, userId, tmpl, subparams, ccAdmin)
		if err != nil {
			reportError(err, "failed to send email", fmt.Sprintf("userid=%d, tmpl=%s, subparam=%v", userId, tmpl, subparams))
		}
	}()
}
