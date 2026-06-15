// SPDX-License-Identifier: MIT

package toolbox

// catalog.go — the curated, cross-platform CLI tool catalog (M956), seeded from
// the operator's real host inventory plus the common developer/ops toolkit.
// Each tool lists ordered install recipes per GOOS; ResolveInstall picks the
// first whose manager is present. Recipes use non-interactive flags so the web
// UI can run them unattended. Linux apt recipes assume root (the daemon's user);
// they surface a clear error otherwise rather than prompting.

// ── recipe helpers (keep the catalog compact) ──────────────────────────────

func wg(id string) Recipe {
	return Recipe{
		Manager: "winget",
		Install: []string{"winget", "install", "-e", "--id", id, "--accept-package-agreements", "--accept-source-agreements", "--silent"},
		Upgrade: []string{"winget", "upgrade", "-e", "--id", id, "--accept-package-agreements", "--accept-source-agreements", "--silent"},
	}
}
func ch(pkg string) Recipe {
	return Recipe{Manager: "choco", Install: []string{"choco", "install", pkg, "-y"}, Upgrade: []string{"choco", "upgrade", pkg, "-y"}}
}
func sc(pkg string) Recipe {
	return Recipe{Manager: "scoop", Install: []string{"scoop", "install", pkg}, Upgrade: []string{"scoop", "update", pkg}}
}
func br(f string) Recipe {
	return Recipe{Manager: "brew", Install: []string{"brew", "install", f}, Upgrade: []string{"brew", "upgrade", f}}
}
func ap(p string) Recipe {
	return Recipe{Manager: "apt", Install: []string{"apt-get", "install", "-y", p}, Upgrade: []string{"apt-get", "install", "-y", "--only-upgrade", p}}
}
func dn(p string) Recipe {
	return Recipe{Manager: "dnf", Install: []string{"dnf", "install", "-y", p}, Upgrade: []string{"dnf", "upgrade", "-y", p}}
}
func pac(p string) Recipe {
	return Recipe{Manager: "pacman", Install: []string{"pacman", "-S", "--noconfirm", p}, Upgrade: []string{"pacman", "-S", "--noconfirm", p}}
}
func pip(p string) Recipe {
	return Recipe{Manager: "pip", Install: []string{"pip", "install", "--user", p}, Upgrade: []string{"pip", "install", "--user", "--upgrade", p}}
}
func npmg(p string) Recipe {
	return Recipe{Manager: "npm", Install: []string{"npm", "install", "-g", p}, Upgrade: []string{"npm", "install", "-g", p}}
}
func cargo(p string) Recipe {
	return Recipe{Manager: "cargo", Install: []string{"cargo", "install", p}, Upgrade: []string{"cargo", "install", "--force", p}}
}

// rec is a small helper to build the per-OS recipe map from variadic groups.
type osRecipes struct {
	windows []Recipe
	darwin  []Recipe
	linux   []Recipe
}

func (o osRecipes) m() map[string][]Recipe {
	out := map[string][]Recipe{}
	if len(o.windows) > 0 {
		out["windows"] = o.windows
	}
	if len(o.darwin) > 0 {
		out["darwin"] = o.darwin
	}
	if len(o.linux) > 0 {
		out["linux"] = o.linux
	}
	return out
}

// Catalog is the full curated tool set.
var Catalog = []Tool{
	// ── search & navigation ──
	{Name: "jq", Category: "data", Description: "Command-line JSON processor — slice, filter, map structured data.",
		Recipes: osRecipes{windows: []Recipe{wg("jqlang.jq"), ch("jq"), sc("jq")}, darwin: []Recipe{br("jq")}, linux: []Recipe{ap("jq"), dn("jq"), pac("jq")}}.m()},
	{Name: "rg", Category: "search", Description: "ripgrep — recursively search code, ~10× faster than grep.",
		Recipes: osRecipes{windows: []Recipe{wg("BurntSushi.ripgrep.MSVC"), ch("ripgrep"), sc("ripgrep")}, darwin: []Recipe{br("ripgrep")}, linux: []Recipe{ap("ripgrep"), dn("ripgrep"), pac("ripgrep")}}.m()},
	{Name: "fd", Category: "search", Description: "A fast, user-friendly alternative to find.",
		BinByOS: map[string]string{"linux": "fdfind"},
		Recipes: osRecipes{windows: []Recipe{wg("sharkdp.fd"), ch("fd"), sc("fd")}, darwin: []Recipe{br("fd")}, linux: []Recipe{ap("fd-find"), dn("fd-find"), pac("fd")}}.m()},
	{Name: "bat", Category: "search", Description: "A cat clone with syntax highlighting and Git integration.",
		BinByOS: map[string]string{"linux": "batcat"},
		Recipes: osRecipes{windows: []Recipe{wg("sharkdp.bat"), ch("bat"), sc("bat")}, darwin: []Recipe{br("bat")}, linux: []Recipe{ap("bat"), dn("bat"), pac("bat")}}.m()},
	{Name: "fzf", Category: "search", Description: "A general-purpose command-line fuzzy finder.",
		Recipes: osRecipes{windows: []Recipe{wg("junegunn.fzf"), ch("fzf"), sc("fzf")}, darwin: []Recipe{br("fzf")}, linux: []Recipe{ap("fzf"), dn("fzf"), pac("fzf")}}.m()},
	{Name: "zoxide", Category: "shell", Description: "A smarter cd — jump to frequently-used directories.",
		Recipes: osRecipes{windows: []Recipe{wg("ajeetdsouza.zoxide"), sc("zoxide")}, darwin: []Recipe{br("zoxide")}, linux: []Recipe{ap("zoxide"), pac("zoxide"), cargo("zoxide")}}.m()},
	{Name: "tree", Category: "shell", Description: "Recursive directory listing as a tree.",
		Recipes: osRecipes{windows: []Recipe{ch("tree"), sc("tree")}, darwin: []Recipe{br("tree")}, linux: []Recipe{ap("tree"), dn("tree"), pac("tree")}}.m()},

	// ── git & TUI ──
	{Name: "lazygit", Category: "vcs", Description: "A simple terminal UI for git commands.",
		Recipes: osRecipes{windows: []Recipe{wg("JesseDuffield.lazygit"), sc("lazygit")}, darwin: []Recipe{br("lazygit")}, linux: []Recipe{pac("lazygit")}}.m()},

	// ── system monitors ──
	{Name: "btop", Category: "shell", Description: "A modern resource monitor (CPU, memory, disk, net).",
		Recipes: osRecipes{windows: []Recipe{sc("btop"), ch("btop")}, darwin: []Recipe{br("btop")}, linux: []Recipe{ap("btop"), dn("btop"), pac("btop")}}.m()},
	{Name: "htop", Category: "shell", Description: "An interactive process viewer.",
		Recipes: osRecipes{darwin: []Recipe{br("htop")}, linux: []Recipe{ap("htop"), dn("htop"), pac("htop")}}.m()},

	// ── media & docs ──
	{Name: "ffmpeg", Category: "media", Description: "Record, convert and stream audio and video.",
		Recipes: osRecipes{windows: []Recipe{wg("Gyan.FFmpeg"), ch("ffmpeg"), sc("ffmpeg")}, darwin: []Recipe{br("ffmpeg")}, linux: []Recipe{ap("ffmpeg"), dn("ffmpeg"), pac("ffmpeg")}}.m()},
	{Name: "pandoc", Category: "media", Description: "Universal document converter (markdown ↔ docx/pdf/html…).",
		Recipes: osRecipes{windows: []Recipe{wg("JohnMacFarlane.Pandoc"), ch("pandoc")}, darwin: []Recipe{br("pandoc")}, linux: []Recipe{ap("pandoc"), dn("pandoc"), pac("pandoc")}}.m()},
	{Name: "magick", Category: "media", Description: "ImageMagick — create, edit and convert images from the CLI.",
		Recipes: osRecipes{windows: []Recipe{wg("ImageMagick.ImageMagick"), ch("imagemagick")}, darwin: []Recipe{br("imagemagick")}, linux: []Recipe{ap("imagemagick"), dn("ImageMagick"), pac("imagemagick")}}.m()},
	{Name: "yt-dlp", Category: "media", Description: "Download video/audio from YouTube and 1000+ sites.",
		Recipes: osRecipes{windows: []Recipe{wg("yt-dlp.yt-dlp"), sc("yt-dlp"), pip("yt-dlp")}, darwin: []Recipe{br("yt-dlp"), pip("yt-dlp")}, linux: []Recipe{pip("yt-dlp"), pac("yt-dlp")}}.m()},
	{Name: "whisper", Category: "ai", Description: "OpenAI Whisper — local speech-to-text transcription.",
		Recipes: osRecipes{windows: []Recipe{pip("openai-whisper")}, darwin: []Recipe{pip("openai-whisper")}, linux: []Recipe{pip("openai-whisper")}}.m()},

	// ── build toolchain ──
	{Name: "make", Category: "build", Description: "The classic build automation tool.",
		Recipes: osRecipes{windows: []Recipe{ch("make"), sc("make")}, darwin: []Recipe{br("make")}, linux: []Recipe{ap("make"), dn("make"), pac("make")}}.m()},
	{Name: "cmake", Category: "build", Description: "Cross-platform build-system generator.",
		Recipes: osRecipes{windows: []Recipe{wg("Kitware.CMake"), ch("cmake")}, darwin: []Recipe{br("cmake")}, linux: []Recipe{ap("cmake"), dn("cmake"), pac("cmake")}}.m()},
	{Name: "gcc", Category: "build", Description: "The GNU C/C++ compiler.",
		Recipes: osRecipes{windows: []Recipe{ch("mingw"), sc("gcc")}, darwin: []Recipe{br("gcc")}, linux: []Recipe{ap("gcc"), dn("gcc"), pac("gcc")}}.m()},

	// ── archives ──
	{Name: "7z", Category: "archive", Description: "7-Zip — high-ratio archiver for many formats.",
		BinByOS: map[string]string{"linux": "7z"},
		Recipes: osRecipes{windows: []Recipe{wg("7zip.7zip"), ch("7zip"), sc("7zip")}, darwin: []Recipe{br("p7zip")}, linux: []Recipe{ap("p7zip-full"), dn("p7zip"), pac("p7zip")}}.m()},
	{Name: "zip", Category: "archive", Description: "Create .zip archives.",
		Recipes: osRecipes{windows: []Recipe{ch("zip")}, darwin: []Recipe{br("zip")}, linux: []Recipe{ap("zip"), dn("zip"), pac("zip")}}.m()},
	{Name: "unzip", Category: "archive", Description: "Extract .zip archives.",
		Recipes: osRecipes{windows: []Recipe{ch("unzip")}, darwin: []Recipe{br("unzip")}, linux: []Recipe{ap("unzip"), dn("unzip"), pac("unzip")}}.m()},

	// ── network ──
	{Name: "wget", Category: "net", Description: "Non-interactive network downloader.",
		Recipes: osRecipes{windows: []Recipe{wg("JernejSimoncic.Wget"), ch("wget"), sc("wget")}, darwin: []Recipe{br("wget")}, linux: []Recipe{ap("wget"), dn("wget"), pac("wget")}}.m()},
	{Name: "openssl", Category: "net", Description: "TLS/crypto toolkit — keys, certs, hashing.",
		Recipes: osRecipes{windows: []Recipe{wg("ShiningLight.OpenSSL.Light"), ch("openssl")}, darwin: []Recipe{br("openssl")}, linux: []Recipe{ap("openssl"), dn("openssl"), pac("openssl")}}.m()},
	{Name: "rsync", Category: "net", Description: "Fast incremental file transfer/sync.",
		Recipes: osRecipes{windows: []Recipe{ch("rsync")}, darwin: []Recipe{br("rsync")}, linux: []Recipe{ap("rsync"), dn("rsync"), pac("rsync")}}.m()},

	// ── databases ──
	{Name: "sqlite3", Category: "data", Description: "The SQLite command-line shell.",
		Recipes: osRecipes{windows: []Recipe{wg("SQLite.SQLite"), ch("sqlite")}, darwin: []Recipe{br("sqlite")}, linux: []Recipe{ap("sqlite3"), dn("sqlite"), pac("sqlite")}}.m()},
	{Name: "psql", Category: "data", Description: "The PostgreSQL interactive terminal.",
		Recipes: osRecipes{windows: []Recipe{ch("postgresql")}, darwin: []Recipe{br("libpq")}, linux: []Recipe{ap("postgresql-client"), dn("postgresql"), pac("postgresql")}}.m()},
	{Name: "redis-cli", Category: "data", Description: "The Redis command-line client.",
		Recipes: osRecipes{darwin: []Recipe{br("redis")}, linux: []Recipe{ap("redis-tools"), dn("redis"), pac("redis")}}.m()},
	{Name: "mysql", Category: "data", Description: "The MySQL/MariaDB command-line client.",
		Recipes: osRecipes{darwin: []Recipe{br("mysql-client")}, linux: []Recipe{ap("mysql-client"), dn("mysql"), pac("mariadb-clients")}}.m()},
	{Name: "mongosh", Category: "data", Description: "The MongoDB Shell.",
		Recipes: osRecipes{windows: []Recipe{wg("MongoDB.Shell")}, darwin: []Recipe{br("mongosh")}, linux: []Recipe{npmg("mongosh")}}.m()},

	// ── cloud & k8s ──
	{Name: "aws", Category: "cloud", Description: "AWS Command Line Interface.",
		Recipes: osRecipes{windows: []Recipe{wg("Amazon.AWSCLI"), ch("awscli")}, darwin: []Recipe{br("awscli")}, linux: []Recipe{pip("awscli"), ap("awscli")}}.m()},
	{Name: "gcloud", Category: "cloud", Description: "Google Cloud CLI.",
		Recipes: osRecipes{windows: []Recipe{wg("Google.CloudSDK")}, darwin: []Recipe{br("google-cloud-sdk")}, linux: []Recipe{}}.m()},
	{Name: "az", Category: "cloud", Description: "Microsoft Azure CLI.",
		Recipes: osRecipes{windows: []Recipe{wg("Microsoft.AzureCLI"), ch("azure-cli")}, darwin: []Recipe{br("azure-cli")}, linux: []Recipe{}}.m()},
	{Name: "kubectl", Category: "cloud", Description: "Kubernetes command-line tool.", VersionArgs: []string{"version", "--client"},
		Recipes: osRecipes{windows: []Recipe{wg("Kubernetes.kubectl"), ch("kubernetes-cli")}, darwin: []Recipe{br("kubernetes-cli")}, linux: []Recipe{ap("kubectl"), pac("kubectl")}}.m()},
	{Name: "helm", Category: "cloud", Description: "The Kubernetes package manager.",
		Recipes: osRecipes{windows: []Recipe{wg("Helm.Helm"), ch("kubernetes-helm")}, darwin: []Recipe{br("helm")}, linux: []Recipe{pac("helm")}}.m()},

	// ── runtimes & language package managers ──
	{Name: "pwsh", Category: "runtime", Description: "PowerShell 7 (cross-platform).",
		Recipes: osRecipes{windows: []Recipe{wg("Microsoft.PowerShell"), ch("powershell-core")}, darwin: []Recipe{br("powershell")}, linux: []Recipe{}}.m()},
	{Name: "pipx", Category: "pkgmgr", Description: "Install and run Python CLI apps in isolated envs.",
		Recipes: osRecipes{windows: []Recipe{pip("pipx")}, darwin: []Recipe{br("pipx"), pip("pipx")}, linux: []Recipe{ap("pipx"), pip("pipx")}}.m()},
	{Name: "poetry", Category: "pkgmgr", Description: "Python dependency management and packaging.",
		Recipes: osRecipes{windows: []Recipe{pip("poetry")}, darwin: []Recipe{br("poetry"), pip("poetry")}, linux: []Recipe{pip("poetry")}}.m()},

	// ── AI ──
	{Name: "ollama", Category: "ai", Description: "Run open LLMs locally.",
		Recipes: osRecipes{windows: []Recipe{wg("Ollama.Ollama")}, darwin: []Recipe{br("ollama")}, linux: []Recipe{}}.m()},
}
