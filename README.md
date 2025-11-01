```
 ██████████                          █████████  ████                               
░░███░░░░███                        ███░░░░░███░░███                               
 ░███   ░░███  ██████  █████ █████ ███     ░░░  ░███   ██████   ██████   ████████  
 ░███    ░███ ███░░███░░███ ░░███ ░███          ░███  ███░░███ ░░░░░███ ░░███░░███ 
 ░███    ░███░███████  ░███  ░███ ░███          ░███ ░███████   ███████  ░███ ░███ 
 ░███    ███ ░███░░░   ░░███ ███  ░░███     ███ ░███ ░███░░░   ███░░███  ░███ ░███ 
 ██████████  ░░██████   ░░█████    ░░█████████  █████░░██████ ░░████████ ████ █████
░░░░░░░░░░    ░░░░░░     ░░░░░      ░░░░░░░░░  ░░░░░  ░░░░░░   ░░░░░░░░ ░░░░ ░░░░░ 
```

# DevClean

TUI program to help developers clean up common development output directories and build artifacts. Program is in the alpha stage. Expect lots of bugs!

## Installation

1.  Download the latest release from the [GitHub Releases](https://github.com/arkorty/DevClean/releases) page.
2.  (Optional but recommended) Verify the checksum of the downloaded binary.
3.  Move the binary to a directory in your system's `PATH`.
    - For Linux/macOS: `mv devclean ~/.local/bin/` or `sudo mv devclean /usr/local/bin/`
    - For Windows: Move `devclean.exe` to a folder that is in your system's PATH.

## Usage

### Basic Usage
```bash
# Scan current directory
devclean

# Scan a specific directory
devclean /path/to/your/project
```

### Controls

| Key | Action |
|-----|--------|
| `Space` or `Enter` | Toggle selection of a directory |
| `d` | Delete all selected directories |
| `D` | Select all SAFE directories and confirm deletion |
| `r` | Refresh the directory scan |
| `q` or `Ctrl+C` | Quit the application |

## Features

-   **Recursive Scan**: Finds common development output folders deep within your project structure.
-   **Interactive UI**: A simple checkbox interface to select directories for removal.
-   **Safety System**: Directories are categorized as **Safe**, **Moderate**, or **Risky** to prevent accidental deletion.
-   **Detailed Information**: Shows the path, a description, and the calculated size for each directory.
-   **Fast & Efficient**: Uses concurrency for quick scanning with timeout protection.

## Disclaimer

This software is provided "as is", without warranty of any kind, express or implied. Use at your own risk.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

