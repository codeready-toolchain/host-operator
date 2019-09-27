// Code generated for package nstemplatetiers by go-bindata DO NOT EDIT. (@generated)
// sources:
// deploy/templates/nstemplatetiers/advanced-code.yaml
// deploy/templates/nstemplatetiers/advanced-dev.yaml
// deploy/templates/nstemplatetiers/advanced-stage.yaml
// deploy/templates/nstemplatetiers/basic-code.yaml
// deploy/templates/nstemplatetiers/basic-dev.yaml
// deploy/templates/nstemplatetiers/basic-stage.yaml
// deploy/templates/nstemplatetiers/metadata.yaml
package nstemplatetiers

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)
type asset struct {
	bytes []byte
	info  os.FileInfo
}

type bindataFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
}

// Name return file name
func (fi bindataFileInfo) Name() string {
	return fi.name
}

// Size return file size
func (fi bindataFileInfo) Size() int64 {
	return fi.size
}

// Mode return file mode
func (fi bindataFileInfo) Mode() os.FileMode {
	return fi.mode
}

// Mode return file modify time
func (fi bindataFileInfo) ModTime() time.Time {
	return fi.modTime
}

// IsDir return file whether a directory
func (fi bindataFileInfo) IsDir() bool {
	return fi.mode&os.ModeDir != 0
}

// Sys return file is sys mode
func (fi bindataFileInfo) Sys() interface{} {
	return nil
}

var _advancedCodeYaml = []byte(`apiVersion: template.openshift.io/v1
kind: Template
metadata:
  labels:
    provider: codeready-toolchain
  name: advanced-code
objects:
- apiVersion: v1
  kind: Namespace
  metadata:
    annotations:
      openshift.io/description: ${USERNAME}-code
      openshift.io/display-name: ${USERNAME}-code
      openshift.io/requester: ${USERNAME}
    labels:
      provider: codeready-toolchain
    name: ${USERNAME}-code
parameters:
- name: USERNAME
  required: true`)

func advancedCodeYamlBytes() ([]byte, error) {
	return _advancedCodeYaml, nil
}

func advancedCodeYaml() (*asset, error) {
	bytes, err := advancedCodeYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "advanced-code.yaml", size: 462, mode: os.FileMode(420), modTime: time.Unix(1568993327, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _advancedDevYaml = []byte(`apiVersion: template.openshift.io/v1
kind: Template
metadata:
  labels:
    provider: codeready-toolchain
  name: advanced-dev
objects:
- apiVersion: v1
  kind: Namespace
  metadata:
    annotations:
      openshift.io/description: ${USERNAME}-dev
      openshift.io/display-name: ${USERNAME}-dev
      openshift.io/requester: ${USERNAME}
    labels:
      provider: codeready-toolchain
    name: ${USERNAME}-dev
parameters:
- name: USERNAME
  required: true`)

func advancedDevYamlBytes() ([]byte, error) {
	return _advancedDevYaml, nil
}

func advancedDevYaml() (*asset, error) {
	bytes, err := advancedDevYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "advanced-dev.yaml", size: 458, mode: os.FileMode(420), modTime: time.Unix(1568993327, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _advancedStageYaml = []byte(`apiVersion: template.openshift.io/v1
kind: Template
metadata:
  labels:
    provider: codeready-toolchain
  name: advanced-stage
objects:
- apiVersion: v1
  kind: Namespace
  metadata:
    annotations:
      openshift.io/description: ${USERNAME}-stage
      openshift.io/display-name: ${USERNAME}-stage
      openshift.io/requester: ${USERNAME}
    labels:
      provider: codeready-toolchain
    name: ${USERNAME}-stage
parameters:
- name: USERNAME
  required: true`)

func advancedStageYamlBytes() ([]byte, error) {
	return _advancedStageYaml, nil
}

func advancedStageYaml() (*asset, error) {
	bytes, err := advancedStageYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "advanced-stage.yaml", size: 466, mode: os.FileMode(420), modTime: time.Unix(1568993327, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _basicCodeYaml = []byte(`apiVersion: template.openshift.io/v1
kind: Template
metadata:
  labels:
    provider: codeready-toolchain
  name: basic-code
objects:
- apiVersion: v1
  kind: Namespace
  metadata:
    annotations:
      openshift.io/description: ${USERNAME}-code
      openshift.io/display-name: ${USERNAME}-code
      openshift.io/requester: ${USERNAME}
    labels:
      provider: codeready-toolchain
    name: ${USERNAME}-code
parameters:
- name: USERNAME
  required: true`)

func basicCodeYamlBytes() ([]byte, error) {
	return _basicCodeYaml, nil
}

func basicCodeYaml() (*asset, error) {
	bytes, err := basicCodeYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "basic-code.yaml", size: 459, mode: os.FileMode(420), modTime: time.Unix(1568993327, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _basicDevYaml = []byte(`apiVersion: template.openshift.io/v1
kind: Template
metadata:
  labels:
    provider: codeready-toolchain
  name: basic-dev
objects:
- apiVersion: v1
  kind: Namespace
  metadata:
    annotations:
      openshift.io/description: ${USERNAME}-dev
      openshift.io/display-name: ${USERNAME}-dev
      openshift.io/requester: ${USERNAME}
    labels:
      provider: codeready-toolchain
    name: ${USERNAME}-dev
parameters:
- name: USERNAME
  required: true`)

func basicDevYamlBytes() ([]byte, error) {
	return _basicDevYaml, nil
}

func basicDevYaml() (*asset, error) {
	bytes, err := basicDevYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "basic-dev.yaml", size: 455, mode: os.FileMode(420), modTime: time.Unix(1568993327, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _basicStageYaml = []byte(`apiVersion: template.openshift.io/v1
kind: Template
metadata:
  labels:
    provider: codeready-toolchain
  name: basic-stage
objects:
- apiVersion: v1
  kind: Namespace
  metadata:
    annotations:
      openshift.io/description: ${USERNAME}-stage
      openshift.io/display-name: ${USERNAME}-stage
      openshift.io/requester: ${USERNAME}
    labels:
      provider: codeready-toolchain
    name: ${USERNAME}-stage
parameters:
- name: USERNAME
  required: true`)

func basicStageYamlBytes() ([]byte, error) {
	return _basicStageYaml, nil
}

func basicStageYaml() (*asset, error) {
	bytes, err := basicStageYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "basic-stage.yaml", size: 463, mode: os.FileMode(420), modTime: time.Unix(1568993327, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _metadataYaml = []byte(`advanced-code: e3f140c
advanced-dev: e3f140c
advanced-stage: e3f140c
basic-code: e3f140c
basic-dev: e3f140c
basic-stage: e3f140c
`)

func metadataYamlBytes() ([]byte, error) {
	return _metadataYaml, nil
}

func metadataYaml() (*asset, error) {
	bytes, err := metadataYamlBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "metadata.yaml", size: 129, mode: os.FileMode(420), modTime: time.Unix(1569596221, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

// Asset loads and returns the asset for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func Asset(name string) ([]byte, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		a, err := f()
		if err != nil {
			return nil, fmt.Errorf("Asset %s can't read by error: %v", name, err)
		}
		return a.bytes, nil
	}
	return nil, fmt.Errorf("Asset %s not found", name)
}

// MustAsset is like Asset but panics when Asset would return an error.
// It simplifies safe initialization of global variables.
func MustAsset(name string) []byte {
	a, err := Asset(name)
	if err != nil {
		panic("asset: Asset(" + name + "): " + err.Error())
	}

	return a
}

// AssetInfo loads and returns the asset info for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func AssetInfo(name string) (os.FileInfo, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		a, err := f()
		if err != nil {
			return nil, fmt.Errorf("AssetInfo %s can't read by error: %v", name, err)
		}
		return a.info, nil
	}
	return nil, fmt.Errorf("AssetInfo %s not found", name)
}

// AssetNames returns the names of the assets.
func AssetNames() []string {
	names := make([]string, 0, len(_bindata))
	for name := range _bindata {
		names = append(names, name)
	}
	return names
}

// _bindata is a table, holding each asset generator, mapped to its name.
var _bindata = map[string]func() (*asset, error){
	"advanced-code.yaml":  advancedCodeYaml,
	"advanced-dev.yaml":   advancedDevYaml,
	"advanced-stage.yaml": advancedStageYaml,
	"basic-code.yaml":     basicCodeYaml,
	"basic-dev.yaml":      basicDevYaml,
	"basic-stage.yaml":    basicStageYaml,
	"metadata.yaml":       metadataYaml,
}

// AssetDir returns the file names below a certain
// directory embedded in the file by go-bindata.
// For example if you run go-bindata on data/... and data contains the
// following hierarchy:
//     data/
//       foo.txt
//       img/
//         a.png
//         b.png
// then AssetDir("data") would return []string{"foo.txt", "img"}
// AssetDir("data/img") would return []string{"a.png", "b.png"}
// AssetDir("foo.txt") and AssetDir("notexist") would return an error
// AssetDir("") will return []string{"data"}.
func AssetDir(name string) ([]string, error) {
	node := _bintree
	if len(name) != 0 {
		cannonicalName := strings.Replace(name, "\\", "/", -1)
		pathList := strings.Split(cannonicalName, "/")
		for _, p := range pathList {
			node = node.Children[p]
			if node == nil {
				return nil, fmt.Errorf("Asset %s not found", name)
			}
		}
	}
	if node.Func != nil {
		return nil, fmt.Errorf("Asset %s not found", name)
	}
	rv := make([]string, 0, len(node.Children))
	for childName := range node.Children {
		rv = append(rv, childName)
	}
	return rv, nil
}

type bintree struct {
	Func     func() (*asset, error)
	Children map[string]*bintree
}

var _bintree = &bintree{nil, map[string]*bintree{
	"advanced-code.yaml":  &bintree{advancedCodeYaml, map[string]*bintree{}},
	"advanced-dev.yaml":   &bintree{advancedDevYaml, map[string]*bintree{}},
	"advanced-stage.yaml": &bintree{advancedStageYaml, map[string]*bintree{}},
	"basic-code.yaml":     &bintree{basicCodeYaml, map[string]*bintree{}},
	"basic-dev.yaml":      &bintree{basicDevYaml, map[string]*bintree{}},
	"basic-stage.yaml":    &bintree{basicStageYaml, map[string]*bintree{}},
	"metadata.yaml":       &bintree{metadataYaml, map[string]*bintree{}},
}}

// RestoreAsset restores an asset under the given directory
func RestoreAsset(dir, name string) error {
	data, err := Asset(name)
	if err != nil {
		return err
	}
	info, err := AssetInfo(name)
	if err != nil {
		return err
	}
	err = os.MkdirAll(_filePath(dir, filepath.Dir(name)), os.FileMode(0755))
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(_filePath(dir, name), data, info.Mode())
	if err != nil {
		return err
	}
	err = os.Chtimes(_filePath(dir, name), info.ModTime(), info.ModTime())
	if err != nil {
		return err
	}
	return nil
}

// RestoreAssets restores an asset under the given directory recursively
func RestoreAssets(dir, name string) error {
	children, err := AssetDir(name)
	// File
	if err != nil {
		return RestoreAsset(dir, name)
	}
	// Dir
	for _, child := range children {
		err = RestoreAssets(dir, filepath.Join(name, child))
		if err != nil {
			return err
		}
	}
	return nil
}

func _filePath(dir, name string) string {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	return filepath.Join(append([]string{dir}, strings.Split(cannonicalName, "/")...)...)
}
