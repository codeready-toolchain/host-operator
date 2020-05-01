package notificationtemplates

import (
	"strings"

	"github.com/codeready-toolchain/host-operator/pkg/templates/assets"

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

// Option is a function to set assets
type Option func(asset *assets.Assets)

// WithAssets is used to override the default assets
func WithAssets(a assets.Assets) Option {
	return func(assets *assets.Assets) {
		templates = nil
		assets.Names = a.Names
		assets.Asset = a.Asset
	}
}

// WithEmptyCache clears the cache of templates
func WithEmptyCache() Option {
	return func(_ *assets.Assets) {
		templates = nil
	}
}

// GetNotificationTemplate returns a notification subject, body and a boolean
// indicating whether or not a template was found. Otherwise, an error will be returned
func GetNotificationTemplate(name string, opts ...Option) (*NotificationTemplate, bool, error) {

	assets := assets.NewAssets(AssetNames, Asset)
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

func loadTemplates(assets assets.Assets) (map[string]NotificationTemplate, error) {
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
