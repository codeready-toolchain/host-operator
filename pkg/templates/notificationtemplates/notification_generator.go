package notificationtemplates

import (
	"strings"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"

	"github.com/pkg/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("notification-templates")

// template: a template containing template name and template content
type template struct {
	templateName string
	content      string
}

func GetNotificationTemplates(namespace string, assets Assets) ([]toolchainv1alpha1.NotificationTemplate, error) {
	templates, err := loadTemplates(assets)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get notification templates")
	}

	return newNotificationTemplates(namespace, templates)
}

func loadTemplates(assets Assets) (map[string][]template, error) {
	paths := assets.Names()
	templates := make(map[string][]template)
	for _, path := range paths {
		content, err := assets.Asset(path)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to load templates")
		}
		segments := strings.Split(path, "/")
		if len(segments) != 2 {
			return nil, errors.Wrapf(errors.New("unable to load templates"), "path must contain directory and file")
		}
		directoryName := segments[0]
		filename := segments[1]

		if directoryName == "" || filename == "" {
			return nil, errors.Wrapf(errors.New("unable to load templates"), "directory name and filename cannot be empty")
		}
		tmpl := template{
			templateName: filename,
			content:      string(content),
		}
		templates[directoryName] = append(templates[directoryName], tmpl)
	}

	return templates, nil
}

func newNotificationTemplates(namespace string, data map[string][]template) ([]toolchainv1alpha1.NotificationTemplate, error) {
	var notificationTemplates []toolchainv1alpha1.NotificationTemplate
	for name, templates := range data {
		var subject string
		var content string
		for _, template := range templates {
			switch template.templateName {
			case "notification.html":
				content = template.content
			case "subject.txt":
				subject = template.content
			default:
				return nil, errors.Wrapf(errors.New("unable to load templates"), "must contain notification.html and subject.txt")
			}
		}

		notificationTemplates = append(notificationTemplates, toolchainv1alpha1.NotificationTemplate{
			ObjectMeta: v1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: toolchainv1alpha1.NotificationTemplateSpec{
				Subject: subject,
				Content: content,
			},
		})
	}

	return notificationTemplates, nil
}
