/*
Copyright 2017 Heptio Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package loader

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/heptio/sonobuoy/pkg/plugin"
	"github.com/heptio/sonobuoy/pkg/plugin/driver/daemonset"
	"github.com/heptio/sonobuoy/pkg/plugin/driver/job"
	"github.com/heptio/sonobuoy/pkg/plugin/manifest"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	kuberuntime "k8s.io/apimachinery/pkg/runtime"
)

// LoadAllPlugins loads all plugins by finding plugin definitions in the given
// directory, taking a user's plugin selections, and a sonobuoy phone home
// address (host:port) and returning all of the active, configured plugins for
// this sonobuoy run.
func LoadAllPlugins(namespace string, searchPath []string, selections []plugin.Selection) (ret []plugin.Interface, err error) {
	pluginFiles := []string{}
	for _, dir := range searchPath {
		wd, _ := os.Getwd()
		logrus.Infof("Scanning plugins in %v (pwd: %v)", dir, wd)

		// We only care about configured plugin directories that exist,
		// since we may have a broad search path.
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			logrus.Infof("Directory (%v) does not exist", dir)
			continue
		}

		files, err := findPlugins(dir)
		if err != nil {
			return []plugin.Interface{}, errors.Wrapf(err, "couldn't scan %v for plugins", dir)
		}
		pluginFiles = append(pluginFiles, files...)
	}

	pluginDefs := []*manifest.Manifest{}
	for _, file := range pluginFiles {
		pluginDef, err := loadDefinition(file)
		if err != nil {
			return []plugin.Interface{}, errors.Wrapf(err, "couldn't load plugin definition %v", file)
		}
		pluginDefs = append(pluginDefs, pluginDef)
	}

	pluginDefs = filterPluginDef(pluginDefs, selections)

	plugins := []plugin.Interface{}
	for _, def := range pluginDefs {
		pluginIface, err := loadPlugin(def, namespace)
		if err != nil {
			return nil, errors.Wrapf(err, "couldn't load plugin %v", def.SonobuoyConfig.PluginName)
		}
		plugins = append(plugins, pluginIface)
	}

	return plugins, nil
}

func findPlugins(dir string) ([]string, error) {
	return filepath.Glob(path.Join(dir, "*.yml"))
}

func loadDefinition(file string) (*manifest.Manifest, error) {
	bytes, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, errors.Wrapf(err, "couldn't open plugin definition %v", file)
	}

	var def manifest.Manifest
	if err = kuberuntime.DecodeInto(manifest.Decoder, bytes, &def); err != nil {
		return nil, errors.Wrapf(err, "couldn't decode json for plugin definition %v", file)
	}

	return &def, nil
}

func loadPlugin(def *manifest.Manifest, namespace string) (plugin.Interface, error) {
	pluginDef := plugin.Definition{
		Name:       def.SonobuoyConfig.PluginName,
		ResultType: def.SonobuoyConfig.ResultType,
		Spec:       def.Spec,
	}

	switch def.SonobuoyConfig.Driver {
	case "Job":
		return job.NewPlugin(pluginDef, namespace), nil
	case "DaemonSet":
		return daemonset.NewPlugin(pluginDef, namespace), nil
	default:
		return nil, fmt.Errorf("unknown driver %q for plugin %v",
			def.SonobuoyConfig.Driver, def.SonobuoyConfig.PluginName)
	}
}

func filterPluginDef(defs []*manifest.Manifest, selections []plugin.Selection) []*manifest.Manifest {
	m := make(map[string]bool)
	for _, selection := range selections {
		m[selection.Name] = true
	}

	filtered := []*manifest.Manifest{}
	for _, def := range defs {
		if m[def.SonobuoyConfig.PluginName] {
			filtered = append(filtered, def)
		}
	}
	return filtered
}

// From https://stackoverflow.com/questions/40737122/convert-yaml-to-json-without-struct-golang
func convert(i interface{}) interface{} {
	switch x := i.(type) {
	case map[interface{}]interface{}:
		m2 := map[string]interface{}{}
		for k, v := range x {
			m2[k.(string)] = convert(v)
		}
		return m2
	case []interface{}:
		for i, v := range x {
			x[i] = convert(v)
		}
	}
	return i
}
