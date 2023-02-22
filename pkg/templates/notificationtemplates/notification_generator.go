package notificationtemplates

import (
	"strings"

	"github.com/codeready-toolchain/host-operator/pkg/templates/assets"
	"github.com/pkg/errors"
)

const (
	sandboxNotificationEnvironment   = "sandbox"
	stonesoupNotificationEnvironment = "stonesoup"
)

var notificationTemplates map[string]map[string]NotificationTemplate

var SandboxUserProvisioned, _, _ = GetNotificationTemplate("userprovisioned", sandboxNotificationEnvironment)
var SandboxUserDeactivated, _, _ = GetNotificationTemplate("userdeactivated", sandboxNotificationEnvironment)
var SandboxUserDeactivating, _, _ = GetNotificationTemplate("userdeactivating", sandboxNotificationEnvironment)
var SandboxIdlerTriggered, _, _ = GetNotificationTemplate("idlertriggered", sandboxNotificationEnvironment)

// NotificationTemplate contains the template subject and content
type NotificationTemplate struct {
	Subject string
	Content string
	Name    string
}

// GetNotificationTemplate returns a notification subject, body and a boolean
// indicating whether or not a template was found. Otherwise, an error will be returned
func GetNotificationTemplate(name string, notificationEnvironment string) (*NotificationTemplate, bool, error) {
	templates, err := loadTemplates()
	if err != nil {
		return nil, false, errors.Wrap(err, "unable to get notification templates")
	}
	template, found := templates[notificationEnvironment][name]
	return &template, found, nil
}

func templatesForAssets(assets assets.Assets) (map[string]map[string]NotificationTemplate, error) {
	paths := assets.Names()
	notificationTemplates = make(map[string]map[string]NotificationTemplate)
	notificationTemplates[sandboxNotificationEnvironment] = make(map[string]NotificationTemplate)
	notificationTemplates[stonesoupNotificationEnvironment] = make(map[string]NotificationTemplate)
	for _, path := range paths {
		content, err := assets.Asset(path)
		if err != nil {
			return nil, err
		}
		segments := strings.Split(path, "/")
		if len(segments) != 3 {
			return nil, errors.Wrapf(errors.New("path must contain env, directory and file"), "unable to load templates")
		}
		env := segments[0]
		directoryName := segments[1]
		filename := segments[2]

		template := notificationTemplates[env][directoryName]
		template.Name = directoryName
		switch filename {
		case "notification.html":
			template.Content = string(content)
			notificationTemplates[env][directoryName] = template
		case "subject.txt":
			template.Subject = string(content)
			notificationTemplates[env][directoryName] = template
		default:
			return nil, errors.Wrapf(errors.New("must contain notification.html and subject.txt"), "unable to load templates")
		}
	}

	return notificationTemplates, nil
}

func loadTemplates() (map[string]map[string]NotificationTemplate, error) {
	if notificationTemplates != nil {
		return notificationTemplates, nil
	}
	return templatesForAssets(assets.NewAssets(AssetNames, Asset))
}
