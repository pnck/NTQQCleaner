// Command winrespatch 用 winres 库（wails 的资源嵌入同款，docs/10
// §2.1）后封 Windows exe：
//
//   - 以 raw XML 整体替换 RT_MANIFEST——渲染后的
//     build/windows/wails.exe.manifest（含 consoleAllocationPolicy，
//     wails 的嵌入阶段会丢弃该元素，只能后封）；
//   - 补/覆盖 icon（build/windows/icon.ico）与版本信息（渲染后的
//     build/windows/info.json，模板解析规则与 wails setDefaults 一致）。
//
// 纯 Go PE 操作：可在 linux 交叉构建机上处理 windows exe。裸 build
// 路径由此补齐资源，bundle 路径幂等覆盖。运行于仓库根（Taskfile
// 调用）：winrespatch <exe>。
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/tc-hib/winres"
	"github.com/tc-hib/winres/version"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: winrespatch <exe>")
		os.Exit(2)
	}
	if err := patch(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "winrespatch:", err)
		os.Exit(1)
	}
}

// projectData 与 wails internal/project 的 setDefaults 对齐
// （docs/10 §1：缺省值逐字段一致）。
type projectData struct {
	Name string `json:"name"`
	Info struct {
		CompanyName    string  `json:"companyName"`
		ProductName    string  `json:"productName"`
		ProductVersion string  `json:"productVersion"`
		Copyright      *string `json:"copyright"`
		Comments       *string `json:"comments"`
	} `json:"info"`
}

func loadProject(root string) (projectData, error) {
	p := projectData{}
	raw, err := os.ReadFile(filepath.Join(root, "wails.json"))
	if err != nil {
		return p, fmt.Errorf("read wails.json: %w", err)
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, fmt.Errorf("parse wails.json: %w", err)
	}
	// wails setDefaults 同款缺省（internal/project/project.go）
	if p.Name == "" {
		p.Name = "wailsapp"
	}
	if p.Info.CompanyName == "" {
		p.Info.CompanyName = p.Name
	}
	if p.Info.ProductName == "" {
		p.Info.ProductName = p.Name
	}
	if p.Info.ProductVersion == "" {
		p.Info.ProductVersion = "1.0.0"
	}
	if p.Info.Copyright == nil {
		v := "Copyright........."
		p.Info.Copyright = &v
	}
	if p.Info.Comments == nil {
		v := "Built using Wails (https://wails.io)"
		p.Info.Comments = &v
	}
	return p, nil
}

// render 渲染 build 目录下的 wails 模板（{{.Name}} 等占位符）。
func render(root, file string, data any) ([]byte, error) {
	tmpl, err := template.ParseFiles(filepath.Join(root, file))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", file, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render %s: %w", file, err)
	}
	return buf.Bytes(), nil
}

// infoJSON 镜像 build/windows/info.json 模板的输出形状。
type infoJSON struct {
	Fixed struct {
		FileVersion string `json:"file_version"`
	} `json:"fixed"`
	Info map[string]struct {
		ProductVersion  string `json:"ProductVersion"`
		CompanyName     string `json:"CompanyName"`
		FileDescription string `json:"FileDescription"`
		LegalCopyright  string `json:"LegalCopyright"`
		ProductName     string `json:"ProductName"`
		Comments        string `json:"Comments"`
	} `json:"info"`
}

func patch(exe string) error {
	root := "."
	p, err := loadProject(root)
	if err != nil {
		return err
	}

	manifestXML, err := render(root, "build/windows/wails.exe.manifest", p)
	if err != nil {
		return err
	}
	infoData, err := render(root, "build/windows/info.json", p)
	if err != nil {
		return err
	}
	var info infoJSON
	if err := json.Unmarshal(infoData, &info); err != nil {
		return fmt.Errorf("parse rendered info.json: %w", err)
	}

	in, err := os.Open(exe)
	if err != nil {
		return err
	}
	rs, err := winres.LoadFromEXE(in)
	if err != nil {
		if err == winres.ErrNoResources {
			rs = &winres.ResourceSet{}
		} else {
			in.Close()
			return fmt.Errorf("load resources from %s: %w", exe, err)
		}
	}

	// manifest：raw XML 直塞（不解析、不丢元素）
	if err := rs.Set(winres.RT_MANIFEST, winres.ID(1), winres.LCIDDefault, manifestXML); err != nil {
		in.Close()
		return err
	}
	// icon：与 bundle 路径同源（build/windows/icon.ico）
	icoFile, err := os.Open(filepath.Join(root, "build/windows/icon.ico"))
	if err == nil {
		ico, err := winres.LoadICO(icoFile)
		icoFile.Close()
		if err == nil {
			if err := rs.SetIcon(winres.RT_ICON, ico); err != nil {
				in.Close()
				return err
			}
		}
	}
	// 版本信息：与 wails 解析规则同源（渲染后的 info.json）
	vi := version.Info{}
	vi.SetFileVersion(info.Fixed.FileVersion)
	for lang, st := range info.Info {
		lid := uint16(0)
		if _, err := fmt.Sscanf(lang, "%x", &lid); err == nil {
			for key, val := range map[string]string{
				"ProductVersion":  st.ProductVersion,
				"CompanyName":     st.CompanyName,
				"FileDescription": st.FileDescription,
				"LegalCopyright":  st.LegalCopyright,
				"ProductName":     st.ProductName,
				"Comments":        st.Comments,
			} {
				if val != "" {
					_ = vi.Set(lid, key, val)
				}
			}
		}
	}
	rs.SetVersionInfo(vi)

	// 写回：临时文件 + rename（原子替换）
	tmp := exe + ".winres.tmp"
	out, err := os.Create(tmp)
	if err != nil {
		in.Close()
		return err
	}
	if err := rs.WriteToEXE(out, in); err != nil {
		out.Close()
		in.Close()
		os.Remove(tmp)
		return fmt.Errorf("write %s: %w", exe, err)
	}
	in.Close()
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, exe); err != nil {
		os.Remove(tmp)
		return err
	}
	fmt.Printf("winrespatch: %s patched (manifest+icon+version)\n", exe)
	return nil
}
