# kensington-trackball-scroll

A simple Linux utility that converts trackball movement into scroll events.

## Getting Started

1. Clone the repo

```bash
git@github.com:builtbylarry/kensington-trackball-scroll.git
```

2. Build the project

```bash
go build
```

3. Run the program

```bash
# Auto-detect trackball and use default settings
./trackball-scroll

# Specify a device manually
./trackball-scroll -device /dev/input/event0

# Adjust sensitivity and dead zone
./trackball-scroll -sensitivity 0.5 -deadzone 3
```

> You may need root privileges for your device to be detected

## Options

- `-sensitivity`: Scroll sensitivity (default: 0.3)
- `-deadzone`: Dead zone for ignoring small movements (default: 2)
- `-device`: Device path or "auto" for auto-detection (default: "auto")

## Contributing

Any contributions are greatly appreciated!
If you have a suggestion that would make Distill better, please open an issue and assign it the "enhancement" tag.

To make changes, create a new feature branch:

`git checkout -b feature/MyFeatureName`

After making your changes, submit a pull request via the [GitHub web panel](https://github.com/builtbylarry/kensington-trackball-scroll/compare).

> Note that making public contributions to this repo means you accept the LICENSE in place, and are contributing code that also respects that same license

## License

Distributed under the MIT License. See `LICENSE.txt` for more information.
