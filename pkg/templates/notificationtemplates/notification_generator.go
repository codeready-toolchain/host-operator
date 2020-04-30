package notificationtemplates

import (
	"strings"

	"github.com/pkg/errors"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("notification-templates")

// NotificationTemplate contains the template subject and content
type NotificationTemplate struct {
	Subject string
	Content string
}

// GetNotificationTemplates returns a notification subject and body or an error
func GetNotificationTemplates(name string, assets Assets) (NotificationTemplate, error) {
	templates, err := loadTemplates(assets)
	if err != nil {
		return NotificationTemplate{}, errors.Wrap(err, "unable to get notification templates")
	}
	return templates[name], nil
}

func loadTemplates(assets Assets) (map[string]NotificationTemplate, error) {
	paths := assets.Names()
	templates := make(map[string]NotificationTemplate)

	for _, path := range paths {
		content, err := assets.Asset(path)
		if err != nil {
			return nil, err
		}
		segments := strings.Split(path, "/")
		if len(segments) != 2 {
			return nil, errors.Wrapf(errors.New("path must contain directory and file"), "unable to load templates")
		}
		directoryName := segments[0]
		filename := segments[1]

		if directoryName == "" || filename == "" {
			return nil, errors.Wrapf(errors.New("directory name and filename cannot be empty"), "unable to load templates")
		}

		template := templates[directoryName]
		switch filename {
		case "notification.html":
			template.Content = string(content)
			templates[directoryName] = template
		case "subject.txt":
			template.Subject = string(content)
			templates[directoryName] = template
		default:
			return nil, errors.Wrapf(errors.New("must contain notification.html and subject.txt"), "unable to load templates")
		}
	}
	return templates, nil
}
