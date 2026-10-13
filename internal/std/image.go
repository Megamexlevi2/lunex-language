package std

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"strings"
	"sync"

	"lunex/internal/runtime"

	"github.com/disintegration/imaging"
)

const maxImageDimension = 16384
const maxImagePixels = int64(16777216)

type lunexImage struct {
	img *image.NRGBA
	mu  sync.Mutex
}

var imageRegistry = struct {
	sync.Mutex
	m map[*runtime.Value]*lunexImage
}{m: make(map[*runtime.Value]*lunexImage)}

func ImageModule() *runtime.Value {
	return runtime.ObjectVal(map[string]*runtime.Value{
		"create": runtime.FuncVal(&runtime.Function{Name: "create", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 || len(args) == 3 || len(args) == 4 || len(args) > 6 {
				return runtime.Null, fmt.Errorf("create(width, height[, r, g, b[, a]]) requires 2, 5, or 6 arguments")
			}
			width, err := imageIntArg(args, 0, "width")
			if err != nil {
				return runtime.Null, err
			}
			height, err := imageIntArg(args, 1, "height")
			if err != nil {
				return runtime.Null, err
			}
			if err := validateImageDimensions(width, height); err != nil {
				return runtime.Null, err
			}
			fill := color.NRGBA{}
			if len(args) > 2 {
				fill.A = 255
				channels := []*uint8{&fill.R, &fill.G, &fill.B}
				channelNames := []string{"red", "green", "blue"}
				for i, target := range channels {
					if len(args) > i+2 {
						value, err := imageColorArg(args, i+2, channelNames[i])
						if err != nil {
							return runtime.Null, err
						}
						*target = value
					}
				}
				if len(args) > 5 {
					value, err := imageColorArg(args, 5, "alpha")
					if err != nil {
						return runtime.Null, err
					}
					fill.A = value
				}
			}
			return imageObject(&lunexImage{img: imaging.New(width, height, fill)}), nil
		}}),
		"open": runtime.FuncVal(&runtime.Function{Name: "open", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 1 || args[0] == nil || args[0].Tag != runtime.TypeString {
				return runtime.Null, fmt.Errorf("open(path) requires a file path")
			}
			path := args[0].StrVal
			file, err := os.Open(path)
			if err != nil {
				return runtime.Null, err
			}
			config, _, configErr := image.DecodeConfig(file)
			closeErr := file.Close()
			if configErr != nil {
				return runtime.Null, configErr
			}
			if closeErr != nil {
				return runtime.Null, closeErr
			}
			if err := validateImageDimensions(config.Width, config.Height); err != nil {
				return runtime.Null, err
			}
			decoded, err := imaging.Open(path, imaging.AutoOrientation(true))
			if err != nil {
				return runtime.Null, err
			}
			if err := validateImageDimensions(decoded.Bounds().Dx(), decoded.Bounds().Dy()); err != nil {
				return runtime.Null, err
			}
			return imageObject(&lunexImage{img: imageNRGBA(decoded)}), nil
		}}),
		"decode": runtime.FuncVal(&runtime.Function{Name: "decode", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.Null, fmt.Errorf("decode(bytes) requires image data")
			}
			data, err := imageBytes(args[0])
			if err != nil {
				return runtime.Null, err
			}
			config, _, err := image.DecodeConfig(bytes.NewReader(data))
			if err != nil {
				return runtime.Null, err
			}
			if err := validateImageDimensions(config.Width, config.Height); err != nil {
				return runtime.Null, err
			}
			decoded, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
			if err != nil {
				return runtime.Null, err
			}
			if err := validateImageDimensions(decoded.Bounds().Dx(), decoded.Bounds().Dy()); err != nil {
				return runtime.Null, err
			}
			return imageObject(&lunexImage{img: imageNRGBA(decoded)}), nil
		}}),
	})
}

func imageObject(li *lunexImage) *runtime.Value {
	var obj *runtime.Value
	methods := map[string]*runtime.Value{
		"pixel": runtime.FuncVal(&runtime.Function{Name: "pixel", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.Null, fmt.Errorf("pixel(x, y) requires coordinates")
			}
			x, err := imageIntArg(args, 0, "x")
			if err != nil {
				return runtime.Null, err
			}
			y, err := imageIntArg(args, 1, "y")
			if err != nil {
				return runtime.Null, err
			}
			li.mu.Lock()
			defer li.mu.Unlock()
			if !image.Pt(x, y).In(li.img.Bounds()) {
				return runtime.Null, fmt.Errorf("pixel coordinates are outside the image")
			}
			p := li.img.NRGBAAt(x, y)
			return runtime.ObjectVal(map[string]*runtime.Value{
				"r": runtime.NumberVal(float64(p.R)),
				"g": runtime.NumberVal(float64(p.G)),
				"b": runtime.NumberVal(float64(p.B)),
				"a": runtime.NumberVal(float64(p.A)),
			}), nil
		}}),
		"setPixel": runtime.FuncVal(&runtime.Function{Name: "setPixel", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 5 {
				return runtime.Null, fmt.Errorf("setPixel(x, y, r, g, b, a?) requires coordinates and RGB values")
			}
			x, err := imageIntArg(args, 0, "x")
			if err != nil {
				return runtime.Null, err
			}
			y, err := imageIntArg(args, 1, "y")
			if err != nil {
				return runtime.Null, err
			}
			channels := [4]uint8{0, 0, 0, 255}
			channelNames := []string{"red", "green", "blue"}
			for i := 0; i < 3; i++ {
				channels[i], err = imageColorArg(args, i+2, channelNames[i])
				if err != nil {
					return runtime.Null, err
				}
			}
			if len(args) > 5 {
				channels[3], err = imageColorArg(args, 5, "alpha")
				if err != nil {
					return runtime.Null, err
				}
			}
			li.mu.Lock()
			defer li.mu.Unlock()
			if !image.Pt(x, y).In(li.img.Bounds()) {
				return runtime.Null, fmt.Errorf("pixel coordinates are outside the image")
			}
			li.img.SetNRGBA(x, y, color.NRGBA{R: channels[0], G: channels[1], B: channels[2], A: channels[3]})
			return obj, nil
		}}),
		"resize": runtime.FuncVal(&runtime.Function{Name: "resize", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.Null, fmt.Errorf("resize(width, height) requires dimensions")
			}
			width, err := imageIntArg(args, 0, "width")
			if err != nil {
				return runtime.Null, err
			}
			height, err := imageIntArg(args, 1, "height")
			if err != nil {
				return runtime.Null, err
			}
			if err := validateImageDimensions(width, height); err != nil {
				return runtime.Null, err
			}
			li.mu.Lock()
			li.img = imaging.Resize(li.img, width, height, imaging.Lanczos)
			updateImageDimensions(obj, li.img)
			li.mu.Unlock()
			return obj, nil
		}}),
		"crop": runtime.FuncVal(&runtime.Function{Name: "crop", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 4 {
				return runtime.Null, fmt.Errorf("crop(x, y, width, height) requires a rectangle")
			}
			x, err := imageIntArg(args, 0, "x")
			if err != nil {
				return runtime.Null, err
			}
			y, err := imageIntArg(args, 1, "y")
			if err != nil {
				return runtime.Null, err
			}
			width, err := imageIntArg(args, 2, "width")
			if err != nil {
				return runtime.Null, err
			}
			height, err := imageIntArg(args, 3, "height")
			if err != nil {
				return runtime.Null, err
			}
			if err := validateImageDimensions(width, height); err != nil {
				return runtime.Null, err
			}
			li.mu.Lock()
			defer li.mu.Unlock()
			bounds := li.img.Bounds()
			if x < 0 || y < 0 || x > bounds.Dx()-width || y > bounds.Dy()-height {
				return runtime.Null, fmt.Errorf("crop rectangle is outside the image")
			}
			li.img = imaging.Crop(li.img, image.Rect(x, y, x+width, y+height))
			updateImageDimensions(obj, li.img)
			return obj, nil
		}}),
		"rotate": runtime.FuncVal(&runtime.Function{Name: "rotate", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 1 || args[0] == nil || args[0].Tag != runtime.TypeNumber || !imageFinite(args[0].ToNumber()) {
				return runtime.Null, fmt.Errorf("rotate(degrees) requires a finite angle")
			}
			angle := math.Mod(args[0].ToNumber(), 360)
			li.mu.Lock()
			defer li.mu.Unlock()
			width, height := li.img.Bounds().Dx(), li.img.Bounds().Dy()
			radians := angle * math.Pi / 180
			rotatedWidth := int(math.Ceil(math.Abs(float64(width)*math.Cos(radians)) + math.Abs(float64(height)*math.Sin(radians)) - 1e-9))
			rotatedHeight := int(math.Ceil(math.Abs(float64(height)*math.Cos(radians)) + math.Abs(float64(width)*math.Sin(radians)) - 1e-9))
			if err := validateImageDimensions(rotatedWidth, rotatedHeight); err != nil {
				return runtime.Null, err
			}
			li.img = imaging.Rotate(li.img, angle, color.NRGBA{})
			updateImageDimensions(obj, li.img)
			return obj, nil
		}}),
		"flipH": runtime.FuncVal(&runtime.Function{Name: "flipH", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			li.mu.Lock()
			li.img = imaging.FlipH(li.img)
			li.mu.Unlock()
			return obj, nil
		}}),
		"flipV": runtime.FuncVal(&runtime.Function{Name: "flipV", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			li.mu.Lock()
			li.img = imaging.FlipV(li.img)
			li.mu.Unlock()
			return obj, nil
		}}),
		"gray": runtime.FuncVal(&runtime.Function{Name: "gray", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			li.mu.Lock()
			li.img = imaging.Grayscale(li.img)
			li.mu.Unlock()
			return obj, nil
		}}),
		"invert": runtime.FuncVal(&runtime.Function{Name: "invert", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			li.mu.Lock()
			li.img = imaging.Invert(li.img)
			li.mu.Unlock()
			return obj, nil
		}}),
		"brightness": runtime.FuncVal(&runtime.Function{Name: "brightness", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			percentage, err := imagePercentageArg(args, "brightness")
			if err != nil {
				return runtime.Null, err
			}
			li.mu.Lock()
			li.img = imaging.AdjustBrightness(li.img, percentage)
			li.mu.Unlock()
			return obj, nil
		}}),
		"contrast": runtime.FuncVal(&runtime.Function{Name: "contrast", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			percentage, err := imagePercentageArg(args, "contrast")
			if err != nil {
				return runtime.Null, err
			}
			li.mu.Lock()
			li.img = imaging.AdjustContrast(li.img, percentage)
			li.mu.Unlock()
			return obj, nil
		}}),
		"saturation": runtime.FuncVal(&runtime.Function{Name: "saturation", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			percentage, err := imagePercentageArg(args, "saturation")
			if err != nil {
				return runtime.Null, err
			}
			li.mu.Lock()
			li.img = imaging.AdjustSaturation(li.img, percentage)
			li.mu.Unlock()
			return obj, nil
		}}),
		"paste": runtime.FuncVal(&runtime.Function{Name: "paste", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 3 {
				return runtime.Null, fmt.Errorf("paste(source, x, y, opacity?) requires an image and coordinates")
			}
			source := extractImage(args[0])
			if source == nil {
				return runtime.Null, fmt.Errorf("paste source must be an image")
			}
			x, err := imageIntArg(args, 1, "x")
			if err != nil {
				return runtime.Null, err
			}
			y, err := imageIntArg(args, 2, "y")
			if err != nil {
				return runtime.Null, err
			}
			opacity := 1.0
			if len(args) > 3 {
				if args[3] == nil || args[3].Tag != runtime.TypeNumber {
					return runtime.Null, fmt.Errorf("paste opacity must be a number between 0 and 1")
				}
				opacity = args[3].ToNumber()
			}
			if !imageFinite(opacity) || opacity < 0 || opacity > 1 {
				return runtime.Null, fmt.Errorf("paste opacity must be between 0 and 1")
			}
			source.mu.Lock()
			sourceImage := imageNRGBA(source.img)
			source.mu.Unlock()
			li.mu.Lock()
			li.img = imaging.Overlay(li.img, sourceImage, image.Pt(x, y), opacity)
			li.mu.Unlock()
			return obj, nil
		}}),
		"blur": runtime.FuncVal(&runtime.Function{Name: "blur", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			sigma, err := imageSigmaArg(args, "blur")
			if err != nil {
				return runtime.Null, err
			}
			li.mu.Lock()
			li.img = imaging.Blur(li.img, sigma)
			li.mu.Unlock()
			return obj, nil
		}}),
		"sharpen": runtime.FuncVal(&runtime.Function{Name: "sharpen", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			sigma, err := imageSigmaArg(args, "sharpen")
			if err != nil {
				return runtime.Null, err
			}
			li.mu.Lock()
			li.img = imaging.Sharpen(li.img, sigma)
			li.mu.Unlock()
			return obj, nil
		}}),
		"encode": runtime.FuncVal(&runtime.Function{Name: "encode", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) > 2 {
				return runtime.Null, fmt.Errorf("encode(format?, quality?) accepts at most 2 arguments")
			}
			format := imaging.PNG
			if len(args) > 0 {
				if args[0] == nil || args[0].Tag != runtime.TypeString {
					return runtime.Null, fmt.Errorf("encode format must be a string")
				}
				var err error
				format, err = imageFormat(args[0].ToString())
				if err != nil {
					return runtime.Null, err
				}
			}
			options, err := imageEncodeOptions(args, 1)
			if err != nil {
				return runtime.Null, err
			}
			li.mu.Lock()
			snapshot := imageNRGBA(li.img)
			li.mu.Unlock()
			var out bytes.Buffer
			if err := imaging.Encode(&out, snapshot, format, options...); err != nil {
				return runtime.Null, err
			}
			return bufferObject(newLunexBufferFromBytes(out.Bytes())), nil
		}}),
		"save": runtime.FuncVal(&runtime.Function{Name: "save", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) > 3 {
				return runtime.False, fmt.Errorf("save(path, format?, quality?) accepts at most 3 arguments")
			}
			if len(args) < 1 || args[0] == nil || args[0].Tag != runtime.TypeString {
				return runtime.Null, fmt.Errorf("save(path, format?, quality?) requires a file path")
			}
			path := args[0].StrVal
			options, err := imageEncodeOptions(args, 2)
			if err != nil {
				return runtime.Null, err
			}
			li.mu.Lock()
			snapshot := imageNRGBA(li.img)
			li.mu.Unlock()
			if len(args) > 1 {
				if args[1] == nil || args[1].Tag != runtime.TypeString {
					return runtime.Null, fmt.Errorf("save format must be a string")
				}
				format, err := imageFormat(args[1].ToString())
				if err != nil {
					return runtime.Null, err
				}
				file, err := os.Create(path)
				if err != nil {
					return runtime.False, err
				}
				encodeErr := imaging.Encode(file, snapshot, format, options...)
				closeErr := file.Close()
				if encodeErr != nil {
					return runtime.False, encodeErr
				}
				if closeErr != nil {
					return runtime.False, closeErr
				}
				return runtime.True, nil
			}
			if err := imaging.Save(snapshot, path, options...); err != nil {
				return runtime.False, err
			}
			return runtime.True, nil
		}}),
	}
	obj = runtime.ObjectVal(methods)
	updateImageDimensions(obj, li.img)
	registerImage(obj, li)
	return obj
}

func updateImageDimensions(obj *runtime.Value, img *image.NRGBA) {
	obj.ObjVal["width"] = runtime.NumberVal(float64(img.Bounds().Dx()))
	obj.ObjVal["height"] = runtime.NumberVal(float64(img.Bounds().Dy()))
}

func registerImage(obj *runtime.Value, li *lunexImage) {
	imageRegistry.Lock()
	imageRegistry.m[obj] = li
	imageRegistry.Unlock()
}

func extractImage(obj *runtime.Value) *lunexImage {
	imageRegistry.Lock()
	defer imageRegistry.Unlock()
	return imageRegistry.m[obj]
}

func imageNRGBA(src image.Image) *image.NRGBA {
	bounds := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(dst, dst.Bounds(), src, bounds.Min, draw.Src)
	return dst
}

func imageBytes(value *runtime.Value) ([]byte, error) {
	if value == nil {
		return nil, fmt.Errorf("image data is required")
	}
	switch value.Tag {
	case runtime.TypeString:
		return []byte(value.StrVal), nil
	case runtime.TypeArray:
		out := make([]byte, len(value.ArrVal))
		for i, item := range value.ArrVal {
			if item == nil || item.Tag != runtime.TypeNumber {
				return nil, fmt.Errorf("image byte array must contain numbers")
			}
			n := item.ToNumber()
			if !imageFinite(n) || n < 0 || n > 255 || math.Trunc(n) != n {
				return nil, fmt.Errorf("image byte values must be integers from 0 to 255")
			}
			out[i] = byte(n)
		}
		return out, nil
	case runtime.TypeObject:
		buffer := extractBuffer(value)
		if buffer == nil {
			return nil, fmt.Errorf("decode expects a string, byte array, or std.buffer value")
		}
		buffer.mu.Lock()
		defer buffer.mu.Unlock()
		return append([]byte(nil), buffer.data...), nil
	default:
		return nil, fmt.Errorf("decode expects a string, byte array, or std.buffer value")
	}
}

func imageIntArg(args []*runtime.Value, index int, name string) (int, error) {
	if index >= len(args) || args[index] == nil || args[index].Tag != runtime.TypeNumber {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	n := args[index].ToNumber()
	if !imageFinite(n) || math.Trunc(n) != n || n < float64(-maxImageDimension) || n > float64(maxImageDimension) {
		return 0, fmt.Errorf("%s is outside the supported range", name)
	}
	return int(n), nil
}

func imageColorArg(args []*runtime.Value, index int, name string) (uint8, error) {
	if index >= len(args) || args[index] == nil || args[index].Tag != runtime.TypeNumber {
		return 0, fmt.Errorf("%s must be a number", name)
	}
	n := args[index].ToNumber()
	if !imageFinite(n) || math.Trunc(n) != n || n < 0 || n > 255 {
		return 0, fmt.Errorf("%s must be an integer from 0 to 255", name)
	}
	return uint8(n), nil
}

func imagePercentageArg(args []*runtime.Value, name string) (float64, error) {
	if len(args) == 0 || args[0] == nil || args[0].Tag != runtime.TypeNumber {
		return 0, fmt.Errorf("%s(percentage) requires a number", name)
	}
	value := args[0].ToNumber()
	if !imageFinite(value) || value < -100 || value > 100 {
		return 0, fmt.Errorf("%s percentage must be between -100 and 100", name)
	}
	return value, nil
}

func imageSigmaArg(args []*runtime.Value, name string) (float64, error) {
	if len(args) == 0 || args[0] == nil || args[0].Tag != runtime.TypeNumber {
		return 0, fmt.Errorf("%s(sigma) requires a number", name)
	}
	value := args[0].ToNumber()
	if !imageFinite(value) || value <= 0 || value > 100 {
		return 0, fmt.Errorf("%s sigma must be greater than 0 and at most 100", name)
	}
	return value, nil
}

func imageEncodeOptions(args []*runtime.Value, qualityIndex int) ([]imaging.EncodeOption, error) {
	if len(args) <= qualityIndex {
		return nil, nil
	}
	if args[qualityIndex] == nil || args[qualityIndex].Tag != runtime.TypeNumber {
		return nil, fmt.Errorf("JPEG quality must be an integer from 1 to 100")
	}
	quality := args[qualityIndex].ToNumber()
	if !imageFinite(quality) || math.Trunc(quality) != quality || quality < 1 || quality > 100 {
		return nil, fmt.Errorf("JPEG quality must be an integer from 1 to 100")
	}
	return []imaging.EncodeOption{imaging.JPEGQuality(int(quality))}, nil
}

func imageFormat(value string) (imaging.Format, error) {
	format, err := imaging.FormatFromExtension(strings.ToLower(strings.TrimPrefix(strings.TrimSpace(value), ".")))
	if err != nil {
		return format, fmt.Errorf("unsupported image format %q; use png, jpeg, gif, bmp, or tiff", value)
	}
	return format, nil
}

func validateImageDimensions(width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("image dimensions must be positive")
	}
	if width > maxImageDimension || height > maxImageDimension || int64(width)*int64(height) > maxImagePixels {
		return fmt.Errorf("image dimensions exceed the supported limit")
	}
	return nil
}

func imageFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
