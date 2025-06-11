package notificationtemplates

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"github.com/codeready-toolchain/host-operator/deploy"
	"github.com/pkg/errors"
)

const (
	SandboxTemplateSetName       = "sandbox"
	AppstudioTemplateSetName     = "appstudio"
	UserProvisionedTemplateName  = "userprovisioned"
	UserDeactivatedTemplateName  = "userdeactivated"
	UserDeactivatingTemplateName = "userdeactivating"
	IdlerTriggeredTemplateName   = "idlertriggered"
	rootDirectory                = "templates/notificationtemplates"
)

var notificationTemplates map[string]NotificationTemplate

// NotificationTemplate contains the template subject and content
type NotificationTemplate struct {
	Subject string
	Content string
	Name    string
}

// GetNotificationTemplate returns a NotificationTemplate with the given name for the specified templateSetName. An error will be returned if such a template is not found or there is an error in loading templates.
// This function expects the templates to be organized under rootDirectory as rootDirectory/templateSetName/templateName/notification.html and rootDirectory/templateSetName/templateName/subject.txt
// making the function dependent on the path length.
func GetNotificationTemplate(name string, templateSetName string) (*NotificationTemplate, error) {
	templates, err := loadTemplates(templateSetName)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get notification templates")
	}
	template, found := templates[name]
	if !found {
		return &template, fmt.Errorf("notification template %v not found in %v", name, templateSetName)
	}
	return &template, nil
}

func templatesForAssets(notificationFS embed.FS, root string, setName string) (map[string]NotificationTemplate, error) {
	paths, err := getAllFilenames(&notificationFS, root, setName)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("could not find any emails templates for the environment %v", setName)
	}
	notificationTemplates = make(map[string]NotificationTemplate)
	for _, path := range paths {
		content, err := notificationFS.ReadFile(path)
		if err != nil {
			return nil, err
		}
		segments := strings.Split(path, "/")
		if len(segments) != 5 {
			return nil, errors.Wrapf(errors.New("path must contain directory and file"), "unable to load templates")
		}
		directoryName := segments[3]
		filename := segments[4]

		template := notificationTemplates[directoryName]
		template.Name = directoryName
		switch filename {
		case "notification.html":
			template.Content = string(content)
			notificationTemplates[directoryName] = template
		case "subject.txt":
			template.Subject = string(content)
			notificationTemplates[directoryName] = template
		default:
			return nil, errors.Wrapf(errors.New("must contain notification.html and subject.txt"), "unable to load templates")
		}
	}

	return notificationTemplates, nil
}

func loadTemplates(setName string) (map[string]NotificationTemplate, error) {
	if notificationTemplates != nil {
		return notificationTemplates, nil
	}

	return templatesForAssets(deploy.NotificationTemplateFS, rootDirectory, setName)
}

func getAllFilenames(notificationFS *embed.FS, root string, setName string) (files []string, err error) {

	if err := fs.WalkDir(notificationFS, root, func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}
		if strings.Contains(path, setName) {
			files = append(files, path)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return files, nil
}
