package notificationtemplates

import (
	"strings"

	"github.com/pkg/errors"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("notification-templates")
var templates map[string]NotificationTemplate

// NotificationTemplate contains the template subject and content
type NotificationTemplate struct {
	Subject string
	Content string
}

type Option func(asset *Assets)

func WithAssets(a Assets) Option {
	return func(assets *Assets) {
		templates = nil
		assets.names = a.names
		assets.asset = a.asset
	}
}

func WithEmptyCache() Option {
	return func(_ *Assets) {
		templates = nil
	}
}

// GetNotificationTemplates returns a notification subject and body or an error
func GetNotificationTemplates(name string, opts ...Option) (*NotificationTemplate, bool, error) {

	assets := NewAssets(AssetNames, Asset)
	for _, option := range opts {
		option(&assets)
	}

	templates, err := loadTemplates(assets)
	if err != nil {
		return nil, false, errors.Wrap(err, "unable to get notification templates")
	}
	template, found := templates[name]
	return &template, found, nil
}

func loadTemplates(assets Assets) (map[string]NotificationTemplate, error) {
	if templates != nil {
		return templates, nil
	}

	paths := assets.Names()
	templates = make(map[string]NotificationTemplate)

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
