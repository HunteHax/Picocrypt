package main

/*

Picocrypt v1.23
Copyright (c) Evan Su (https://evansu.cc)
Released under a GNU GPL v3 License
https://github.com/HACKERALERT/Picocrypt

~ In cryptography we trust ~

*/

import (
	_ "embed"

	"archive/zip"
	"bytes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/subtle"

	"fmt"
	"hash"
	"image"
	"image/color"
	"io"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/HACKERALERT/clipboard"
	"github.com/HACKERALERT/dialog"
	"github.com/HACKERALERT/giu"
	"github.com/HACKERALERT/infectious"
	"github.com/HACKERALERT/serpent"
	"github.com/HACKERALERT/zxcvbn-go"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/chacha20"
	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/sha3"
)

// Generic variables
var version = "v1.23"
var window *giu.MasterWindow
var dpi float32
var mode string
var working bool
var recombine bool

// Three variables store the input files
var onlyFiles []string
var onlyFolders []string
var allFiles []string

// Input file variables
var inputLabel = "Drop files and folders into this window."
var inputFile string

// Password variables
var password string
var cPassword string
var passwordStrength int
var passwordState = giu.InputTextFlagsPassword
var passwordStateLabel = "Show"

// Password generator variables
var showGenpass = false
var genpassCopy = true
var genpassLength int32 = 32
var genpassUpper = true
var genpassLower = true
var genpassNums = true
var genpassSymbols = true

// Keyfile variables
var keyfile bool
var keyfiles []string
var keyfileOrderMatters bool
var keyfilePrompt = "None selected."
var showKeyfile bool

// Metadata variables
var metadata string
var metadataPrompt = "Metadata:"
var metadataDisabled bool

// Advanced options
var paranoid bool
var reedsolo bool
var deleteWhenDone bool
var split bool
var splitSize string
var splitUnits = []string{
	"KiB",
	"MiB",
	"GiB",
}
var splitSelected int32 = 1
var compress bool
var keep bool
var kept bool

// Output file variables
var outputFile string

// Status variables
var mainStatus = "Ready."
var mainStatusColor = color.RGBA{0xff, 0xff, 0xff, 0xff}
var popupStatus string

// Progress variables
var progress float32
var progressInfo string
var showProgress bool

// Confirm overwrite variables
var showConfirmation bool

// Reed-Solomon encoders
var rs1, _ = infectious.NewFEC(1, 3) // 1 data shard, 3 total -> 2 parity shards
var rs5, _ = infectious.NewFEC(5, 15)
var rs16, _ = infectious.NewFEC(16, 48)
var rs24, _ = infectious.NewFEC(24, 72)
var rs32, _ = infectious.NewFEC(32, 96)
var rs64, _ = infectious.NewFEC(64, 192)
var rs128, _ = infectious.NewFEC(128, 136)

func draw() {
	giu.SingleWindow().Flags(524351).Layout(
		giu.Custom(func() {
			if showGenpass {
				giu.PopupModal("Generate password:").Flags(6).Layout(
					giu.Row(
						giu.Label("Length:"),
						giu.SliderInt(&genpassLength, 4, 64).Size(giu.Auto),
					),
					giu.Checkbox("Uppercase", &genpassUpper),
					giu.Checkbox("Lowercase", &genpassLower),
					giu.Checkbox("Numbers", &genpassNums),
					giu.Checkbox("Symbols", &genpassSymbols),
					giu.Checkbox("Copy to clipboard", &genpassCopy),
					giu.Row(
						giu.Button("Cancel").Size(100, 0).OnClick(func() {
							giu.CloseCurrentPopup()
							showGenpass = false
						}),
						giu.Button("Generate").Size(100, 0).OnClick(func() {
							tmp := genPassword()
							password = tmp
							cPassword = tmp
							passwordStrength = zxcvbn.PasswordStrength(password, nil).Score
							giu.CloseCurrentPopup()
							showGenpass = false
							giu.Update()
						}),
					),
				).Build()
				giu.OpenPopup("Generate password:")
				giu.Update()
			}

			if showKeyfile {
				giu.PopupModal("Manage keyfiles:").Flags(70).Layout(
					giu.Label("Drag and drop your keyfiles here."),
					giu.Custom(func() {
						if mode != "decrypt" {
							giu.Checkbox("Require correct keyfile order", &keyfileOrderMatters).Build()
							giu.Tooltip("If checked, you will need to drop keyfiles in the correct order.").Build()
						} else if keyfileOrderMatters {
							giu.Label("The correct order of keyfiles is required.").Build()
						}
					}),

					giu.Custom(func() {
						for _, i := range keyfiles {
							giu.Row(
								giu.SmallButton("×").OnClick(func() {
									var tmp []string
									for _, j := range keyfiles {
										if j != i {
											tmp = append(tmp, j)
										}
									}
									keyfiles = tmp
									if len(keyfiles) == 0 {
										keyfilePrompt = "None selected."
									} else if len(keyfiles) == 1 {
										keyfilePrompt = "Using 1 keyfile."
									} else {
										keyfilePrompt = fmt.Sprintf("Using %d keyfiles.", len(keyfiles))
									}
								}),
								giu.Label(filepath.Base(i)),
							).Build()

						}
					}),
					giu.Row(
						giu.Button("Clear").Size(150, 0).OnClick(func() {
							keyfiles = nil
							keyfilePrompt = "None selected."
						}),
						giu.Tooltip("Remove all keyfiles."),
						giu.Button("Done").Size(150, 0).OnClick(func() {
							giu.CloseCurrentPopup()
							showKeyfile = false
						}),
					),
				).Build()
				giu.OpenPopup("Manage keyfiles:")
				giu.Update()
			}

			if showConfirmation {
				giu.PopupModal("Warning:").Flags(6).Layout(
					giu.Label("Output already exists. Overwrite?"),
					giu.Row(
						giu.Button("No").Size(100, 0).OnClick(func() {
							giu.CloseCurrentPopup()
							showConfirmation = false
						}),
						giu.Button("Yes").Size(100, 0).OnClick(func() {
							giu.CloseCurrentPopup()
							showConfirmation = false
							showProgress = true
							giu.Update()
							go func() {
								work()
								working = false
								showProgress = false
								debug.FreeOSMemory()
								giu.Update()
							}()
						}),
					),
				).Build()
				giu.OpenPopup("Warning:")
				giu.Update()
			}

			if showProgress {
				giu.PopupModal(" ").Flags(6).Layout(
					giu.Custom(func() {
						if !working {
							giu.CloseCurrentPopup()
						}
					}),
					giu.Row(
						giu.ProgressBar(progress).Size(280, 0).Overlay(progressInfo),
						giu.Button("Cancel").Size(58, 0).OnClick(func() {
							working = false
						}),
					),
					giu.Label(popupStatus),
				).Build()
				giu.OpenPopup(" ")
				giu.Update()
			}
		}),

		giu.Row(
			giu.Label(inputLabel),
			giu.Custom(func() {
				bw, _ := giu.CalcTextSize("Clear")
				p, _ := giu.GetWindowPadding()
				bw += p * 2
				giu.Dummy(float32(float64(-(bw+p)/dpi)), 0).Build()
				giu.SameLine()
				giu.Style().SetDisabled(len(allFiles) == 0 && len(onlyFiles) == 0).To(
					giu.Button("Clear").Size(bw/dpi, 0).OnClick(resetUI),
					giu.Tooltip("Clear all input files and reset UI state."),
				).Build()
			}),
		),

		giu.Separator(),

		giu.Style().SetDisabled(len(allFiles) == 0 && len(onlyFiles) == 0).To(
			giu.Row(
				giu.Label("Password:"),
				giu.Dummy(-124, 0),
				giu.Style().SetDisabled(mode == "decrypt" && !keyfile).To(
					giu.Label("Keyfiles:"),
				),
			),
			giu.Row(
				giu.Button(passwordStateLabel).Size(54, 0).OnClick(func() {
					if passwordState == giu.InputTextFlagsPassword {
						passwordState = giu.InputTextFlagsNone
						passwordStateLabel = "Hide"
					} else {
						passwordState = giu.InputTextFlagsPassword
						passwordStateLabel = "Show"
					}
				}),

				giu.Button("Clear").Size(54, 0).OnClick(func() {
					password = ""
					cPassword = ""
				}),

				giu.Button("Copy").Size(54, 0).OnClick(func() {
					clipboard.WriteAll(password)
				}),

				giu.Button("Paste").Size(54, 0).OnClick(func() {
					tmp, _ := clipboard.ReadAll()
					password = tmp
					if mode != "decrypt" {
						cPassword = tmp
					}
					passwordStrength = zxcvbn.PasswordStrength(password, nil).Score
					giu.Update()
				}),

				giu.Style().SetDisabled(mode == "decrypt").To(
					giu.Button("Create").Size(54, 0).OnClick(func() {
						showGenpass = true
					}),
				),

				giu.Style().SetDisabled(mode == "decrypt" && !keyfile).To(
					giu.Row(
						giu.Button("Edit").Size(54, 0).OnClick(func() {
							showKeyfile = true
						}),
						giu.Style().SetDisabled(mode == "decrypt").To(
							giu.Button("Create").Size(54, 0).OnClick(func() {
								file, _ := dialog.File().Title("Save keyfile as:").Save()
								if file == "" {
									return
								}
								fout, _ := os.Create(file)
								data := make([]byte, 1048576)
								rand.Read(data)
								fout.Write(data)
								fout.Close()
							}),
						),
					),
				),
			),
			giu.Row(
				giu.InputText(&password).Flags(passwordState).Size(302/dpi).OnChange(func() {
					passwordStrength = zxcvbn.PasswordStrength(password, nil).Score
				}),
				giu.Custom(func() {
					c := giu.GetCanvas()
					p := giu.GetCursorScreenPos()

					var col color.RGBA
					switch passwordStrength {
					case 0:
						col = color.RGBA{0xc8, 0x4c, 0x4b, 0xff}
					case 1:
						col = color.RGBA{0xa9, 0x6b, 0x4b, 0xff}
					case 2:
						col = color.RGBA{0x8a, 0x8a, 0x4b, 0xff}
					case 3:
						col = color.RGBA{0x6b, 0xa9, 0x4b, 0xff}
					case 4:
						col = color.RGBA{0x4c, 0xc8, 0x4b, 0xff}
					}
					if password == "" || mode == "decrypt" {
						col = color.RGBA{0xff, 0xff, 0xff, 0x00}
					}

					path := p.Add(image.Pt(
						int(math.Round(float64(-20*dpi))),
						int(math.Round(float64(12*dpi))),
					))
					c.PathArcTo(path, 6*dpi, -math.Pi/2, float32(passwordStrength+1)/5*2*math.Pi-math.Pi/2, -1)
					c.PathStroke(col, false, 2)
				}),
				giu.Style().SetDisabled(true).To(
					giu.InputText(&keyfilePrompt).Size(giu.Auto),
				),
			),
		),

		giu.Style().SetDisabled(password == "").To(
			giu.Row(
				giu.Style().SetDisabled(mode == "decrypt").To(
					giu.Label("Confirm password:"),
				),
				giu.Dummy(-124, 0),
				giu.Style().SetDisabled(true).To(
					giu.Label("Custom Argon2:"),
				),
			),
		),
		giu.Style().SetDisabled(password == "").To(
			giu.Row(
				giu.Style().SetDisabled(mode == "decrypt").To(
					giu.Row(
						giu.InputText(&cPassword).Flags(passwordState).Size(302/dpi),
						giu.Custom(func() {
							c := giu.GetCanvas()
							p := giu.GetCursorScreenPos()
							col := color.RGBA{0x4c, 0xc8, 0x4b, 0xff}

							if cPassword != password {
								col = color.RGBA{0xc8, 0x4c, 0x4b, 0xff}
							}
							if password == "" || cPassword == "" || mode == "decrypt" {
								col = color.RGBA{0xff, 0xff, 0xff, 0x00}
							}

							path := p.Add(image.Pt(
								int(math.Round(float64(-20*dpi))),
								int(math.Round(float64(12*dpi))),
							))
							c.PathArcTo(path, 6*dpi, 0, 2*math.Pi, -1)
							c.PathStroke(col, false, 2)
						}),
					),
				),
				giu.Style().SetDisabled(true).To(
					giu.Button("W.I.P").Size(giu.Auto, 0),
				),
			),
		),

		giu.Dummy(0, 3),
		giu.Separator(),
		giu.Dummy(0, 0),

		giu.Style().SetDisabled(password == "" || (password != cPassword && mode == "encrypt")).To(
			giu.Label(metadataPrompt),
			giu.Style().SetDisabled(metadataDisabled).To(
				giu.InputText(&metadata).Size(giu.Auto),
			),

			giu.Label("Advanced:"),
			giu.Custom(func() {
				if mode != "decrypt" {
					giu.Row(
						giu.Checkbox("Use paranoid mode", &paranoid),
						giu.Dummy(-221, 0),
						giu.Checkbox("Encode with Reed-Solomon", &reedsolo),
					).Build()
					giu.Row(
						giu.Style().SetDisabled(!(len(allFiles) > 1 || len(onlyFolders) > 0)).To(
							giu.Checkbox("Compress files", &compress),
						),
						giu.Dummy(-221, 0),
						giu.Checkbox("Delete files when complete", &deleteWhenDone),
					).Build()
					giu.Row(
						giu.Checkbox("Split every", &split),
						giu.InputText(&splitSize).Size(55/dpi).Flags(giu.InputTextFlagsCharsHexadecimal).OnChange(func() {
							split = splitSize != ""
						}),
						giu.Combo("##splitter", splitUnits[splitSelected], splitUnits, &splitSelected).Size(52),
					).Build()
				} else {
					giu.Checkbox("Keep decrypted output even if it's corrupted or modified", &keep).Build()
					giu.Checkbox("Delete the encrypted files after a successful decryption", &deleteWhenDone).Build()
				}
			}),

			giu.Label("Save output as:"),
			giu.Custom(func() {
				w, _ := giu.GetAvailableRegion()
				bw, _ := giu.CalcTextSize("Change")
				p, _ := giu.GetWindowPadding()
				bw += p * 2
				dw := w - bw - p
				giu.Style().SetDisabled(true).To(
					giu.InputText(&outputFile).Size(dw / dpi / dpi).Flags(giu.InputTextFlagsReadOnly),
				).Build()
				giu.SameLine()
				giu.Button("Change").Size(bw/dpi, 0).OnClick(func() {
					file, _ := dialog.File().Title("Save as:").Save()
					if file == "" {
						return
					}

					if mode == "encrypt" {
						if len(allFiles) > 1 || len(onlyFolders) > 0 {
							file = strings.TrimSuffix(file, ".zip.pcv")
							file = strings.TrimSuffix(file, ".pcv")
							if !strings.HasSuffix(file, ".zip.pcv") {
								file += ".zip.pcv"
							}
						} else {
							file = strings.TrimSuffix(file, ".pcv")
							ind := strings.Index(inputFile, ".")
							file += inputFile[ind:]
							if !strings.HasSuffix(file, ".pcv") {
								file += ".pcv"
							}
						}
					} else {
						ind := strings.Index(file, ".")
						if ind != -1 {
							file = file[:ind]
						}
						if strings.HasSuffix(inputFile, ".zip.pcv") {
							file += ".zip"
						} else {
							tmp := strings.TrimSuffix(filepath.Base(inputFile), ".pcv")
							tmp = tmp[strings.Index(tmp, "."):]
							file += tmp
						}
					}

					outputFile = file
				}).Build()
				giu.Tooltip("Save the output with a custom path and name.").Build()
			}),

			giu.Dummy(0, 2),
			giu.Separator(),
			giu.Dummy(0, 3),

			giu.Button("Start").Size(giu.Auto, 34).OnClick(func() {
				if keyfile && keyfiles == nil {
					mainStatus = "Please select your keyfiles."
					mainStatusColor = color.RGBA{0xff, 0x00, 0x00, 0xff}
					return
				}
				_, err := os.Stat(outputFile)
				if err == nil {
					showConfirmation = true
					giu.Update()
				} else {
					showProgress = true
					giu.Update()
					go func() {
						work()
						working = false
						showProgress = false
						debug.FreeOSMemory()
						giu.Update()
					}()
				}
			}),
			giu.Style().SetColor(giu.StyleColorText, mainStatusColor).To(
				giu.Label(mainStatus),
			),
		),

		giu.Custom(func() {
			window.SetSize(int(442*dpi), giu.GetCursorPos().Y)
		}),
	)
}

func onDrop(names []string) {
	if showKeyfile {
		keyfiles = append(keyfiles, names...)
		var tmp []string
		for _, i := range keyfiles {
			duplicate := false
			for _, j := range tmp {
				if i == j {
					duplicate = true
				}
			}
			stat, _ := os.Stat(i)
			if !duplicate && !stat.IsDir() {
				tmp = append(tmp, i)
			}
		}
		keyfiles = tmp
		if len(keyfiles) == 1 {
			keyfilePrompt = "Using 1 keyfile."
		} else {
			keyfilePrompt = fmt.Sprintf("Using %d keyfiles.", len(keyfiles))
		}
		return
	}

	// Clear variables
	recombine = false
	onlyFiles = nil
	onlyFolders = nil
	allFiles = nil
	files, folders := 0, 0
	resetUI()

	if len(names) == 1 {
		stat, _ := os.Stat(names[0])
		if stat.IsDir() {
			// Update variables
			mode = "encrypt"
			folders++
			inputLabel = "1 folder selected."

			// Add the folder
			onlyFolders = append(onlyFolders, names[0])

			// Set the input and output paths
			inputFile = filepath.Join(filepath.Dir(names[0]), "Encrypted") + ".zip"
			outputFile = filepath.Join(filepath.Dir(names[0]), "Encrypted") + ".zip.pcv"
		} else {
			files++
			name := filepath.Base(names[0])
			nums := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}
			endsNum := false
			for _, i := range nums {
				if strings.HasSuffix(names[0], i) {
					endsNum = true
				}
			}
			isSplit := strings.Contains(names[0], ".pcv.") && endsNum

			// Decide if encrypting or decrypting
			if strings.HasSuffix(names[0], ".pcv") || isSplit {
				//var err error
				mode = "decrypt"
				inputLabel = name + " (will decrypt)"
				metadataPrompt = "Metadata (read-only):"
				metadataDisabled = true

				if isSplit {
					inputLabel = name + " (will recombine and decrypt)"
					ind := strings.Index(names[0], ".pcv")
					names[0] = names[0][:ind+4]
					inputFile = names[0]
					outputFile = names[0][:ind]
					recombine = true
				} else {
					outputFile = names[0][:len(names[0])-4]
				}

				// Open input file in read-only mode
				var fin *os.File
				if isSplit {
					fin, _ = os.Open(names[0] + ".0")
				} else {
					fin, _ = os.Open(names[0])
				}

				// Use regex to test if input is a valid Picocrypt volume
				tmp := make([]byte, 30)
				fin.Read(tmp)
				if string(tmp[:5]) == "v1.13" {
					resetUI()
					mainStatus = "Please use Picocrypt v1.13 to decrypt this file."
					mainStatusColor = color.RGBA{0xff, 0x00, 0x00, 0xff}
					fin.Close()
					return
				}
				if valid, _ := regexp.Match(`^v\d\.\d{2}.{10}0?\d+`, tmp); !valid && !isSplit {
					resetUI()
					mainStatus = "This doesn't seem to be a Picocrypt volume."
					mainStatusColor = color.RGBA{0xff, 0x00, 0x00, 0xff}
					fin.Close()
					return
				}
				fin.Seek(0, 0)

				// Read metadata and insert into box
				var err error
				tmp = make([]byte, 15)
				fin.Read(tmp)
				tmp, _ = rsDecode(rs5, tmp)
				if string(tmp) == "v1.14" || string(tmp) == "v1.15" || string(tmp) == "v1.16" {
					resetUI()
					mainStatus = "Please use Picocrypt v1.16 to decrypt this file."
					mainStatusColor = color.RGBA{0xff, 0x00, 0x00, 0xff}
					fin.Close()
					return
				}
				if string(tmp) == "v1.17" || string(tmp) == "v1.18" || string(tmp) == "v1.19" ||
					string(tmp) == "v1.20" || string(tmp) == "v1.21" {
					resetUI()
					mainStatus = "Please use Picocrypt v1.21 to decrypt this file."
					mainStatusColor = color.RGBA{0xff, 0x00, 0x00, 0xff}
					fin.Close()
					return
				}
				tmp = make([]byte, 15)
				fin.Read(tmp)
				tmp, err = rsDecode(rs5, tmp)

				if err == nil {
					metadataLength, _ := strconv.Atoi(string(tmp))
					tmp = make([]byte, metadataLength*3)
					fin.Read(tmp)
					metadata = ""

					for i := 0; i < metadataLength*3; i += 3 {
						t, err := rsDecode(rs1, tmp[i:i+3])
						if err != nil {
							metadata = "Metadata is corrupted."
							break
						}
						metadata += string(t)
					}
				} else {
					metadata = "Metadata is corrupted."
				}

				flags := make([]byte, 15)
				fin.Read(flags)
				fin.Close()
				flags, err = rsDecode(rs5, flags)
				if err != nil {
					mainStatus = "Input file is corrupt and cannot be decrypted."
					mainStatusColor = color.RGBA{0xff, 0x00, 0x00, 0xff}
					return
				}

				if flags[1] == 1 {
					keyfile = true
					keyfilePrompt = "Keyfiles required."
				} else {
					keyfilePrompt = "Not applicable."
				}
				if flags[2] == 1 {
					keyfileOrderMatters = true
				}
			} else {
				mode = "encrypt"
				inputLabel = name + " (will encrypt)"
				inputFile = names[0]
				outputFile = names[0] + ".pcv"
			}

			// Add the file
			onlyFiles = append(onlyFiles, names[0])
			inputFile = names[0]
		}
	} else {
		mode = "encrypt"

		// There are multiple dropped items, check each one
		for _, name := range names {
			stat, _ := os.Stat(name)

			// Check if item is a file or a directory
			if stat.IsDir() {
				folders++
				onlyFolders = append(onlyFolders, name)
			} else {
				files++
				onlyFiles = append(onlyFiles, name)
				allFiles = append(allFiles, name)
			}
		}

		if folders == 0 {
			inputLabel = fmt.Sprintf("%d files selected.", files)
		} else if files == 0 {
			inputLabel = fmt.Sprintf("%d folders selected.", files)
		} else {
			if files == 1 && folders > 1 {
				inputLabel = fmt.Sprintf("1 file and %d folders selected.", folders)
			} else if folders == 1 && files > 1 {
				inputLabel = fmt.Sprintf("%d files and 1 folder selected.", files)
			} else if folders == 1 && files == 1 {
				inputLabel = "1 file and 1 folder selected."
			} else {
				inputLabel = fmt.Sprintf("%d files and %d folders selected.", files, folders)
			}
		}

		// Set the input and output paths
		inputFile = filepath.Join(filepath.Dir(names[0]), "Encrypted") + ".zip"
		outputFile = filepath.Join(filepath.Dir(names[0]), "Encrypted") + ".zip.pcv"
	}
	// Recursively add all files to 'allFiles'
	if folders > 0 {
		for _, name := range onlyFolders {
			filepath.Walk(name, func(path string, _ os.FileInfo, _ error) error {
				stat, _ := os.Stat(path)
				if !stat.IsDir() {
					allFiles = append(allFiles, path)
				}
				return nil
			})
		}
	}
}

func work() {
	popupStatus = "Starting..."
	mainStatus = "Working..."
	mainStatusColor = color.RGBA{0xff, 0xff, 0xff, 0xff}
	working = true
	padded := false

	var salt []byte
	var hkdfSalt []byte
	var serpentSalt []byte
	var nonce []byte
	var keyHash []byte
	var _keyHash []byte
	var keyfileKey []byte
	var keyfileHash []byte = make([]byte, 32)
	var _keyfileHash []byte
	var dataMac []byte

	if mode == "encrypt" {
		if compress {
			popupStatus = "Compressing files..."
		} else {
			popupStatus = "Combining files..."
		}

		// "Tar" files together (a .zip file with no compression)
		if len(allFiles) > 1 || len(onlyFolders) > 0 {
			var rootDir string
			if len(onlyFolders) > 0 {
				rootDir = filepath.Dir(onlyFolders[0])
			} else {
				rootDir = filepath.Dir(onlyFiles[0])
			}

			file, err := os.Create(inputFile)
			if err != nil {
				mainStatus = "Access denied by operating system."
				mainStatusColor = color.RGBA{0xff, 0x00, 0x00, 0xff}
				return
			}

			w := zip.NewWriter(file)
			for i, path := range allFiles {
				if !working {
					w.Close()
					file.Close()
					os.Remove(inputFile)
					mainStatus = "Operation cancelled by user."
					mainStatusColor = color.RGBA{0xff, 0xff, 0xff, 0xff}
					return
				}
				progressInfo = fmt.Sprintf("%d/%d", i, len(allFiles))
				progress = float32(i) / float32(len(allFiles))
				giu.Update()
				if path == inputFile {
					continue
				}

				stat, _ := os.Stat(path)
				header, _ := zip.FileInfoHeader(stat)
				header.Name = strings.TrimPrefix(path, rootDir)
				header.Name = filepath.ToSlash(header.Name)
				header.Name = strings.TrimPrefix(header.Name, "/")

				if compress {
					header.Method = zip.Deflate
				} else {
					header.Method = zip.Store
				}
				writer, _ := w.CreateHeader(header)
				file, _ := os.Open(path)
				io.Copy(writer, file)
				file.Close()
			}
			w.Flush()
			w.Close()
			file.Close()
		}
	}

	if recombine {
		popupStatus = "Recombining file..."
		total := 0

		for {
			_, err := os.Stat(fmt.Sprintf("%s.%d", inputFile, total))
			if err != nil {
				break
			}
			total++
		}

		fout, _ := os.Create(inputFile)
		for i := 0; i < total; i++ {
			fin, _ := os.Open(fmt.Sprintf("%s.%d", inputFile, i))
			for {
				data := make([]byte, 1048576)
				read, err := fin.Read(data)
				if err != nil {
					break
				}
				data = data[:read]
				fout.Write(data)
			}
			fin.Close()
			progressInfo = fmt.Sprintf("%d/%d", i, total)
			progress = float32(i) / float32(total)
			giu.Update()
		}
		fout.Close()
		progressInfo = ""
	}

	stat, _ := os.Stat(inputFile)
	total := stat.Size()
	if mode == "decrypt" {
		total -= 786
	}

	// XChaCha20's max message size is 256 GiB
	if total > 256*1073741824 {
		mainStatus = "Total size is larger than 256 GiB, XChaCha20's limit."
		mainStatusColor = color.RGBA{0xff, 0x00, 0x00, 0xff}
		return
	}

	// Open input file in read-only mode
	fin, err := os.Open(inputFile)
	if err != nil {
		mainStatus = "Access denied by operating system."
		mainStatusColor = color.RGBA{0xff, 0x00, 0x00, 0xff}
		return
	}

	var fout *os.File

	// If encrypting, generate values; if decrypting, read values from file
	if mode == "encrypt" {
		popupStatus = "Generating values..."
		giu.Update()

		var err error
		fout, err = os.Create(outputFile)
		if err != nil {
			mainStatus = "Access denied by operating system."
			mainStatusColor = color.RGBA{0xff, 0x00, 0x00, 0xff}
			return
		}

		// Generate random cryptography values
		salt = make([]byte, 16)
		hkdfSalt = make([]byte, 32)
		serpentSalt = make([]byte, 16)
		nonce = make([]byte, 24)

		// Write version to file
		fout.Write(rsEncode(rs5, []byte(version)))

		// Encode the length of the metadata with Reed-Solomon
		metadataLength := []byte(fmt.Sprintf("%05d", len(metadata)))
		metadataLength = rsEncode(rs5, metadataLength)

		// Write the length of the metadata to file
		fout.Write(metadataLength)

		// Reed-Solomon-encode the metadata and write to file
		for _, i := range []byte(metadata) {
			fout.Write(rsEncode(rs1, []byte{i}))
		}

		flags := make([]byte, 5)
		if paranoid {
			flags[0] = 1
		}
		if len(keyfiles) > 0 {
			flags[1] = 1
		}
		if keyfileOrderMatters {
			flags[2] = 1
		}
		if reedsolo {
			flags[3] = 1
		}
		if total%1048576 >= 1048448 {
			flags[4] = 1
		}
		flags = rsEncode(rs5, flags)
		fout.Write(flags)

		// Fill salts and nonce with Go's CSPRNG
		rand.Read(salt)
		rand.Read(hkdfSalt)
		rand.Read(serpentSalt)
		rand.Read(nonce)

		// Encode salt with Reed-Solomon and write to file
		_salt := rsEncode(rs16, salt)
		fout.Write(_salt)

		// Encode HKDF salt with Reed-Solomon and write to file
		_hkdfSalt := rsEncode(rs32, hkdfSalt)
		fout.Write(_hkdfSalt)

		// Encode Serpent salt with Reed-Solomon and write to file
		_serpentSalt := rsEncode(rs16, serpentSalt)
		fout.Write(_serpentSalt)

		// Encode nonce with Reed-Solomon and write to file
		_nonce := rsEncode(rs24, nonce)
		fout.Write(_nonce)

		// Write placeholder for hash of key
		fout.Write(make([]byte, 192))

		// Write placeholder for hash of hash of keyfile
		fout.Write(make([]byte, 96))

		// Write placeholder for HMAC-BLAKE2b/HMAC-SHA3 of file
		fout.Write(make([]byte, 192))
	} else {
		var err1 error
		var err2 error
		var err3 error
		var err4 error
		var err5 error
		var err6 error
		var err7 error
		var err8 error
		var err9 error
		var err10 error

		popupStatus = "Reading values..."
		giu.Update()

		version := make([]byte, 15)
		fin.Read(version)
		_, err1 = rsDecode(rs5, version)

		tmp := make([]byte, 15)
		fin.Read(tmp)
		tmp, err2 = rsDecode(rs5, tmp)
		metadataLength, _ := strconv.Atoi(string(tmp))

		fin.Read(make([]byte, metadataLength*3))

		flags := make([]byte, 15)
		fin.Read(flags)
		flags, err3 = rsDecode(rs5, flags)
		paranoid = flags[0] == 1
		reedsolo = flags[3] == 1
		padded = flags[4] == 1

		salt = make([]byte, 48)
		fin.Read(salt)
		salt, err4 = rsDecode(rs16, salt)

		hkdfSalt = make([]byte, 96)
		fin.Read(hkdfSalt)
		hkdfSalt, err5 = rsDecode(rs32, hkdfSalt)

		serpentSalt = make([]byte, 48)
		fin.Read(serpentSalt)
		serpentSalt, err6 = rsDecode(rs16, serpentSalt)

		nonce = make([]byte, 72)
		fin.Read(nonce)
		nonce, err7 = rsDecode(rs24, nonce)

		_keyHash = make([]byte, 192)
		fin.Read(_keyHash)
		_keyHash, err8 = rsDecode(rs64, _keyHash)

		_keyfileHash = make([]byte, 96)
		fin.Read(_keyfileHash)
		_keyfileHash, err9 = rsDecode(rs32, _keyfileHash)

		dataMac = make([]byte, 192)
		fin.Read(dataMac)
		dataMac, err10 = rsDecode(rs64, dataMac)

		// Is there a better way?
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil ||
			err6 != nil || err7 != nil || err8 != nil || err9 != nil || err10 != nil {
			if keep {
				kept = true
			} else {
				mainStatus = "The header is corrupt and the input file cannot be decrypted."
				mainStatusColor = color.RGBA{0xff, 0x00, 0x00, 0xff}
				fin.Close()
				return
			}
		}
	}

	popupStatus = "Deriving key..."
	progress = 0
	progressInfo = ""
	giu.Update()

	// Derive encryption/decryption keys and subkeys
	var key []byte
	if paranoid {
		key = argon2.IDKey(
			[]byte(password),
			salt,
			8,
			1048576,
			8,
			32,
		)
	} else {
		key = argon2.IDKey(
			[]byte(password),
			salt,
			4,
			1048576,
			4,
			32,
		)
	}

	if !working {
		mainStatus = "Operation cancelled by user."
		mainStatusColor = color.RGBA{0xff, 0xff, 0xff, 0xff}
		if mode == "encrypt" && (len(allFiles) > 1 || len(onlyFolders) > 0) {
			os.Remove(outputFile)
		}
		if recombine {
			os.Remove(inputFile)
		}
		os.Remove(outputFile)
		return
	}

	if len(keyfiles) > 0 || keyfile {
		if keyfileOrderMatters {
			var keysum = sha3.New256()
			for _, path := range keyfiles {
				kin, _ := os.Open(path)
				kstat, _ := os.Stat(path)
				kbytes := make([]byte, kstat.Size())
				kin.Read(kbytes)
				kin.Close()
				keysum.Write(kbytes)
			}
			keyfileKey = keysum.Sum(nil)
			keyfileSha3 := sha3.New256()
			keyfileSha3.Write(keyfileKey)
			keyfileHash = keyfileSha3.Sum(nil)
		} else {
			var keysum []byte
			for _, path := range keyfiles {
				kin, _ := os.Open(path)
				kstat, _ := os.Stat(path)
				kbytes := make([]byte, kstat.Size())
				kin.Read(kbytes)
				kin.Close()
				ksha3 := sha3.New256()
				ksha3.Write(kbytes)
				keyfileKey := ksha3.Sum(nil)
				if keysum == nil {
					keysum = keyfileKey
				} else {
					for i, j := range keyfileKey {
						keysum[i] ^= j
					}
				}
			}
			keyfileKey = keysum
			keyfileSha3 := sha3.New256()
			keyfileSha3.Write(keysum)
			keyfileHash = keyfileSha3.Sum(nil)
		}
	}

	sha3_512 := sha3.New512()
	sha3_512.Write(key)
	keyHash = sha3_512.Sum(nil)

	// Validate password and/or keyfiles
	if mode == "decrypt" {
		keyCorrect := true
		keyfileCorrect := true
		var tmp bool

		keyCorrect = subtle.ConstantTimeCompare(keyHash, _keyHash) != 0
		if keyfile {
			keyfileCorrect = subtle.ConstantTimeCompare(keyfileHash, _keyfileHash) != 0
			tmp = !keyCorrect || !keyfileCorrect
		} else {
			tmp = !keyCorrect
		}

		if tmp || keep {
			if keep {
				kept = true
			} else {
				fin.Close()
				if !keyCorrect {
					mainStatus = "The provided password is incorrect."
				} else {
					if keyfileOrderMatters {
						mainStatus = "Incorrect keyfiles and/or order."
					} else {
						mainStatus = "Incorrect keyfiles."
					}
				}
				mainStatusColor = color.RGBA{0xff, 0x00, 0x00, 0xff}
				key = nil
				if recombine {
					os.Remove(inputFile)
				}
				return
			}
		}

		var err error
		fout, err = os.Create(outputFile)
		if err != nil {
			mainStatus = "Access denied by operating system."
			mainStatusColor = color.RGBA{0xff, 0x00, 0x00, 0xff}
			return
		}
	}

	if len(keyfiles) > 0 || keyfile {
		// XOR key and keyfile
		tmp := key
		key = make([]byte, 32)
		for i := range key {
			key[i] = tmp[i] ^ keyfileKey[i]
		}
	}

	done := 0
	counter := 0
	startTime := time.Now()
	chacha20, _ := chacha20.NewUnauthenticatedCipher(key, nonce)

	// Use HKDF-SHA3 to generate a subkey
	var mac hash.Hash
	subkey := make([]byte, 32)
	hkdf := hkdf.New(sha3.New256, key, hkdfSalt, nil)
	hkdf.Read(subkey)
	if paranoid {
		// HMAC-SHA3
		mac = hmac.New(sha3.New512, subkey)
	} else {
		// Keyed BLAKE2b
		mac, _ = blake2b.New512(subkey)
	}

	// Generate another subkey and cipher (not used unless paranoid mode is checked)
	serpentKey := make([]byte, 32)
	hkdf.Read(serpentKey)
	_serpent, _ := serpent.NewCipher(serpentKey)
	serpentCTR := cipher.NewCTR(_serpent, serpentSalt)

	for {
		if !working {
			mainStatus = "Operation cancelled by user."
			mainStatusColor = color.RGBA{0xff, 0xff, 0xff, 0xff}
			fin.Close()
			fout.Close()
			if mode == "encrypt" && (len(allFiles) > 1 || len(onlyFolders) > 0) {
				os.Remove(outputFile)
			}
			if recombine {
				os.Remove(inputFile)
			}
			os.Remove(outputFile)
			return
		}

		var data []byte
		if mode == "decrypt" && reedsolo {
			data = make([]byte, 1114112)
		} else {
			data = make([]byte, 1048576)
		}

		size, err := fin.Read(data)
		if err != nil {
			break
		}
		data = data[:size]
		_data := make([]byte, len(data))

		// "Actual" encryption is done in the next couple of lines
		if mode == "encrypt" {
			if paranoid {
				serpentCTR.XORKeyStream(_data, data)
				copy(data, _data)
			}

			chacha20.XORKeyStream(_data, data)
			mac.Write(_data)

			if reedsolo {
				copy(data, _data)
				_data = nil
				if len(data) == 1048576 {
					for i := 0; i < 1048576; i += 128 {
						tmp := data[i : i+128]
						tmp = rsEncode(rs128, tmp)
						_data = append(_data, tmp...)
					}
				} else {
					chunks := math.Floor(float64(len(data)) / 128)
					for i := 0; float64(i) < chunks; i++ {
						tmp := data[i*128 : (i+1)*128]
						tmp = rsEncode(rs128, tmp)
						_data = append(_data, tmp...)
					}
					tmp := data[int(chunks*128):]
					_data = append(_data, rsEncode(rs128, pad(tmp))...)
				}
			}
		} else {
			if reedsolo {
				copy(_data, data)
				data = nil
				if len(_data) == 1114112 {
					for i := 0; i < 1114112; i += 136 {
						tmp := _data[i : i+136]
						tmp, err = rsDecode(rs128, tmp)
						if err != nil {
							if keep {
								kept = true
							} else {
								mainStatus = "The input file is too corrupted to decrypt."
								mainStatusColor = color.RGBA{0xff, 0x00, 0x00, 0xff}
								fin.Close()
								fout.Close()
								broken()
								return
							}
						}
						if i == 1113976 && done+1114112 >= int(total) && padded {
							tmp = unpad(tmp)
						}
						data = append(data, tmp...)
					}
				} else {
					chunks := len(_data)/136 - 1
					for i := 0; i < chunks; i++ {
						tmp := _data[i*136 : (i+1)*136]
						tmp, err = rsDecode(rs128, tmp)
						if err != nil {
							if keep {
								kept = true
							} else {
								mainStatus = "The input file is too corrupted to decrypt."
								mainStatusColor = color.RGBA{0xff, 0x00, 0x00, 0xff}
								fin.Close()
								fout.Close()
								broken()
								return
							}
						}
						data = append(data, tmp...)
					}
					tmp := _data[int(chunks)*136:]
					tmp, err = rsDecode(rs128, tmp)
					if err != nil {
						if keep {
							kept = true
						} else {
							mainStatus = "The input file is too corrupted to decrypt."
							mainStatusColor = color.RGBA{0xff, 0x00, 0x00, 0xff}
							fin.Close()
							fout.Close()
							broken()
							return
						}
					}
					tmp = unpad(tmp)
					data = append(data, tmp...)
				}
				_data = make([]byte, len(data))
			}

			mac.Write(data)
			chacha20.XORKeyStream(_data, data)

			if paranoid {
				copy(data, _data)
				serpentCTR.XORKeyStream(_data, data)
			}
		}
		fout.Write(_data)

		// Update stats
		if mode == "decrypt" && reedsolo {
			done += 1114112
		} else {
			done += 1048576
		}
		counter++
		progress = float32(done) / float32(total)
		elapsed := float64(time.Since(startTime)) / math.Pow(10, 9)
		speed := float64(done) / elapsed / math.Pow(10, 6)
		eta := int(math.Floor(float64(total-int64(done)) / (speed * math.Pow(10, 6))))

		if progress > 1 {
			progress = 1
		}

		progressInfo = fmt.Sprintf("%.2f%%", progress*100)
		popupStatus = fmt.Sprintf("Working at %.2f MB/s (ETA: %s)", speed, humanize(eta))
		giu.Update()
	}

	if mode == "encrypt" {
		// Seek back to header and write important data
		fout.Seek(int64(309+len(metadata)*3), 0)
		fout.Write(rsEncode(rs64, keyHash))
		fout.Write(rsEncode(rs32, keyfileHash))
		fout.Write(rsEncode(rs64, mac.Sum(nil)))
	} else {
		// Validate the authenticity of decrypted data
		if subtle.ConstantTimeCompare(mac.Sum(nil), dataMac) == 0 {
			if keep {
				kept = true
			} else {
				fin.Close()
				fout.Close()
				broken()
				return
			}
		}
	}

	fin.Close()
	fout.Close()

	// Split files into chunks
	if split {
		var splitted []string
		popupStatus = "Splitting file..."
		stat, _ := os.Stat(outputFile)
		size := stat.Size()
		finished := 0
		chunkSize, _ := strconv.Atoi(splitSize)

		// User can choose KiB, MiB, and GiB
		if splitSelected == 0 {
			chunkSize *= 1024
		} else if splitSelected == 1 {
			chunkSize *= 1048576
		} else {
			chunkSize *= 1073741824
		}
		chunks := int(math.Ceil(float64(size) / float64(chunkSize)))
		fin, _ := os.Open(outputFile)

		for i := 0; i < chunks; i++ {
			fout, _ := os.Create(fmt.Sprintf("%s.%d", outputFile, i))
			done := 0
			for {
				data := make([]byte, 1048576)
				read, err := fin.Read(data)
				if err != nil {
					break
				}
				if !working {
					fin.Close()
					fout.Close()
					mainStatus = "Operation cancelled by user."
					mainStatusColor = color.RGBA{0xff, 0xff, 0xff, 0xff}

					// If user cancels, remove the unfinished files
					for _, j := range splitted {
						os.Remove(j)
					}
					os.Remove(fmt.Sprintf("%s.%d", outputFile, i))
					os.Remove(outputFile)
					return
				}
				data = data[:read]
				fout.Write(data)
				done += read
				if done >= chunkSize {
					break
				}
			}
			fout.Close()
			finished++
			splitted = append(splitted, fmt.Sprintf("%s.%d", outputFile, i))
			progress = float32(finished) / float32(chunks)
			progressInfo = fmt.Sprintf("%d/%d", finished, chunks)
			giu.Update()
		}
		fin.Close()
		os.Remove(outputFile)
	}

	// Remove the temporary file used to combine a splitted Picocrypt volume
	if recombine {
		os.Remove(inputFile)
	}

	// Delete the temporary zip file if user wishes
	if len(allFiles) > 1 || len(onlyFolders) > 0 {
		os.Remove(inputFile)
	}

	if deleteWhenDone {
		progressInfo = ""
		popupStatus = "Deleted files..."
		giu.Update()
		if mode == "decrypt" {
			if recombine {
				total := 0
				for {
					_, err := os.Stat(fmt.Sprintf("%s.%d", inputFile, total))
					if err != nil {
						break
					}
					os.Remove(fmt.Sprintf("%s.%d", inputFile, total))
					total++
				}
			} else {
				os.Remove(inputFile)
			}
		} else {
			for _, i := range onlyFiles {
				os.Remove(i)
			}
			for _, i := range onlyFolders {
				os.RemoveAll(i)
			}
		}
	}

	resetUI()

	// If user chose to keep a corrupted/modified file, let them know
	if kept {
		mainStatus = "The input file is corrupted and/or modified. Please be careful."
		mainStatusColor = color.RGBA{0xff, 0xff, 0x00, 0xff}
	} else {
		mainStatus = "Completed."
		mainStatusColor = color.RGBA{0x00, 0xff, 0x00, 0xff}
	}

	// Clear UI state
	working = false
	kept = false
	key = nil
	popupStatus = "Ready."
}

// This function is run if an issue occurs during decryption
func broken() {
	mainStatus = "The input file is either corrupted or intentionally modified."
	mainStatusColor = color.RGBA{0xff, 0x00, 0x00, 0xff}
	if recombine {
		os.Remove(inputFile)
	}
	os.Remove(outputFile)
	giu.Update()
}

// Reset the UI to a clean state with nothing selected or checked
func resetUI() {
	mode = ""
	onlyFiles = nil
	onlyFolders = nil
	allFiles = nil
	inputLabel = "Drop files and folders into this window."
	password = ""
	cPassword = ""
	keyfiles = nil
	keyfile = false
	keyfileOrderMatters = false
	keyfilePrompt = "None selected."
	metadata = ""
	metadataPrompt = "Metadata:"
	metadataDisabled = false
	keep = false
	reedsolo = false
	split = false
	splitSize = ""
	splitSelected = 1
	deleteWhenDone = false
	paranoid = false
	compress = false
	inputFile = ""
	outputFile = ""
	progress = 0
	progressInfo = ""
	mainStatus = "Ready."
	mainStatusColor = color.RGBA{0xff, 0xff, 0xff, 0xff}
	giu.Update()
}

// Reed-Solomon encoder
func rsEncode(rs *infectious.FEC, data []byte) []byte {
	var res []byte
	rs.Encode(data, func(s infectious.Share) {
		res = append(res, s.DeepCopy().Data[0])
	})
	return res
}

// Reed-Solomon decoder
func rsDecode(rs *infectious.FEC, data []byte) ([]byte, error) {
	tmp := make([]infectious.Share, rs.Total())
	for i := 0; i < rs.Total(); i++ {
		tmp[i] = infectious.Share{
			Number: i,
			Data:   []byte{data[i]},
		}
	}
	res, err := rs.Decode(nil, tmp)
	if err != nil {
		if rs.Total() == 136 {
			return data[:128], err
		}
		return data[:rs.Total()/3], err
	}
	return res, nil
}

// PKCS7 Pad (for use with Reed-Solomon, not for cryptographic purposes)
func pad(data []byte) []byte {
	padLen := 128 - len(data)%128
	padding := bytes.Repeat([]byte{byte(padLen)}, padLen)
	return append(data, padding...)
}

// PKCS7 Unpad
func unpad(data []byte) []byte {
	length := len(data)
	padLen := int(data[length-1])
	return data[:length-padLen]
}

func genPassword() string {
	chars := ""
	if genpassUpper {
		chars += "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	}
	if genpassLower {
		chars += "abcdefghijklmnopqrstuvwxyz"
	}
	if genpassNums {
		chars += "1234567890"
	}
	if genpassSymbols {
		chars += "-=!@#$^&()_+?"
	}
	if chars == "" {
		return chars
	}
	tmp := make([]byte, genpassLength)
	for i := 0; i < int(genpassLength); i++ {
		j, _ := rand.Int(rand.Reader, new(big.Int).SetUint64(uint64(len(chars))))
		tmp[i] = chars[j.Int64()]
	}
	if genpassCopy {
		clipboard.WriteAll(string(tmp))
	}
	return string(tmp)
}

// Convert seconds to HH:MM:SS
func humanize(seconds int) string {
	hours := int(math.Floor(float64(seconds) / 3600))
	seconds %= 3600
	minutes := int(math.Floor(float64(seconds) / 60))
	seconds %= 60
	hours = int(math.Max(float64(hours), 0))
	minutes = int(math.Max(float64(minutes), 0))
	seconds = int(math.Max(float64(seconds), 0))
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

func main() {
	// Create the master window
	window = giu.NewMasterWindow("Picocrypt", 442, 452, giu.MasterWindowFlagsNotResizable)
	dialog.Init()

	// Set callbacks
	window.SetDropCallback(onDrop)
	window.SetCloseCallback(func() bool {
		return !working
	})

	// Set universal DPI
	dpi = giu.Context.GetPlatform().GetContentScale()

	// Start the UI
	window.Run(draw)
}
