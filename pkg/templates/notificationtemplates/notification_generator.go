package notificationtemplates

import (
	"strings"

	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	commonclient "github.com/codeready-toolchain/toolchain-common/pkg/client"

	"github.com/pkg/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("notification-templates")

func CreateOrUpdateResources(s *runtime.Scheme, client client.Client, namespace string, assets Assets) error {
	templates, err := loadTemplates(assets)
	if err != nil {
		return errors.Wrap(err, "unable to create or update NotificationTemplate")
	}
	notificationCRs, err := newTemplates(namespace, templates)

	for _, notificationCR := range notificationCRs {
		log.Info("creating or updating NSTemplateTier", "namespace", notificationCR.Namespace, "name", notificationCR.Name)
		cl := commonclient.NewApplyClient(client, s)
		createdOrUpdated, err := cl.CreateOrUpdateObject(&notificationCR, true, nil)
		if err != nil {
			return errors.Wrapf(err, "unable to create or update the '%s' NotificationTemplate in namespace '%s'", notificationCR.Name, notificationCR.Namespace)
		}
		if createdOrUpdated {
			log.Info("NotificationTemplate resource created/updated", "namespace", notificationCR.Namespace, "name", notificationCR.Name)
		} else {
			log.Info("NotificationTemplate resource was already up-to-date", "namespace", notificationCR.Namespace, "name", notificationCR.Name)
		}
	}

	return nil
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
		directoryName := segments[0]
		filename := segments[1]

		tmpl := template{
			templateName: filename,
			content:      string(content),
		}
		templates[directoryName] = append(templates[directoryName], tmpl)
	}

	return templates, nil
}

func newTemplates(namespace string, data map[string][]template) ([]toolchainv1alpha1.NotificationTemplate, error) {

	var notificationTemplates []toolchainv1alpha1.NotificationTemplate

	for name, templates := range data {
		subject := ""
		content := ""
		for _, template := range templates {
			if template.templateName == "notification.html" {
				content = template.content
			} else if template.templateName == "subject.txt" {
				subject = template.content
			}
		}

		notificationTemplates = append(notificationTemplates, toolchainv1alpha1.NotificationTemplate{
			TypeMeta: v1.TypeMeta{},
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

// template: a template content and its latest git revision
type template struct {
	templateName string
	content      string
}
