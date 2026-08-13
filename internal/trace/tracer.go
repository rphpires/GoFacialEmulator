package trace

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	// FOLDER_NAME is the name of the folder where logs are stored
	FOLDER_NAME = "logs"

	// LOG_MAX_SIZE is the maximum size of a log file before it's rotated
	LOG_MAX_SIZE = 5 * 1024 * 1024 // 5MB

	// MAX_FILES is the maximum number of log files to keep
	MAX_FILES = 15
)

// LogLevel defines the severity of a log message
type LogLevel int

const (
	// DEBUG is for detailed information
	DEBUG LogLevel = iota
	// INFO is for general information
	INFO
	// WARNING is for warning messages
	WARNING
	// ERROR is for error messages
	ERROR
)

// Tracer handles logging and tracing
type Tracer struct {
	mu       sync.Mutex
	file     *os.File
	htmlFile *os.File
	size     int64
	logger   *log.Logger
}

var (
	// instance is the singleton tracer instance
	instance *Tracer
	// once ensures the tracer is only initialized once
	once sync.Once
)

// NewTracer returns a singleton tracer instance
func NewTracer() *Tracer {
	once.Do(func() {
		instance = &Tracer{}
		if err := instance.init(); err != nil {
			log.Fatalf("Failed to initialize tracer: %v", err)
		}
	})
	return instance
}

// init initializes the tracer
func (t *Tracer) init() error {
	// Check if tracing is disabled
	if fileExists("DisableTraceEnable.txt") || fileExists("DisableTraceIntegraEnable.txt") || fileExists("DisableTrace.txt") {
		t.logger = log.New(os.Stdout, "", log.LstdFlags)
		return nil
	}

	// Create traces directory if it doesn't exist
	if err := os.MkdirAll(FOLDER_NAME, 0755); err != nil {
		return fmt.Errorf("creating traces directory: %w", err)
	}

	// Initialize the log file
	logFilePath := filepath.Join(FOLDER_NAME, "trace.log")
	file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}
	t.file = file

	// Initialize the HTML log file
	htmlFilePath := filepath.Join(FOLDER_NAME, "trace.html")
	htmlFile, err := os.OpenFile(htmlFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.file.Close()
		return fmt.Errorf("opening HTML log file: %w", err)
	}
	t.htmlFile = htmlFile

	// Write HTML header
	if err := t.writeHTMLHeader(); err != nil {
		t.file.Close()
		t.htmlFile.Close()
		return fmt.Errorf("writing HTML header: %w", err)
	}

	// Get file info to track size
	fileInfo, err := os.Stat(logFilePath)
	if err != nil {
		t.file.Close()
		t.htmlFile.Close()
		return fmt.Errorf("getting log file info: %w", err)
	}
	t.size = fileInfo.Size()

	// Create MultiWriter for logging to both file and stdout
	mw := io.MultiWriter(os.Stdout, t.file)
	t.logger = log.New(mw, "", log.LstdFlags)

	// Clean up old log files
	t.cleanOldLogFiles()

	return nil
}

// Close closes the tracer and its log files
func (t *Tracer) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.file != nil {
		t.file.Close()
		t.file = nil
	}

	if t.htmlFile != nil {
		t.htmlFile.Close()
		t.htmlFile = nil
	}
}

// Trace logs a message at the specified level
func (t *Tracer) Trace(level LogLevel, format string, args ...interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Format message
	message := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")

	// Prefix based on log level
	var prefix string
	var color string
	switch level {
	case DEBUG:
		prefix = "DEBUG"
		color = "white"
	case INFO:
		prefix = "INFO"
		color = "white"
	case WARNING:
		prefix = "WARN"
		color = "yellow"
	case ERROR:
		prefix = "ERROR"
		color = "red"
	}

	// Log to stdout/file
	t.logger.Printf("%s - %s - %s", prefix, timestamp, message)

	// Log to HTML file
	if t.htmlFile != nil {
		htmlEntry := fmt.Sprintf("<br></font><font color=\"%s\">%s - %s - %s", color, timestamp, "", message)
		if _, err := t.htmlFile.WriteString(htmlEntry); err != nil {
			log.Printf("Failed to write to HTML log: %v", err)
		}
	}

	// Check if log rotation is needed
	if t.file != nil {
		t.size += int64(len(message) + len(timestamp) + len(prefix) + 5)
		if t.size > LOG_MAX_SIZE {
			t.rotateLogFile()
		}
	}
}

// Debug logs a debug message
func (t *Tracer) Debug(format string, args ...interface{}) {
	t.Trace(DEBUG, format, args...)
}

// Info logs an info message
func (t *Tracer) Info(format string, args ...interface{}) {
	t.Trace(INFO, format, args...)
}

// Warning logs a warning message
func (t *Tracer) Warning(format string, args ...interface{}) {
	t.Trace(WARNING, format, args...)
}

// Error logs an error message
func (t *Tracer) Error(format string, args ...interface{}) {
	t.Trace(ERROR, format, args...)
}

// ReportException logs an exception
func (t *Tracer) ReportException(err error) {
	t.Error("Exception: %v", err)
}

// rotateLogFile rotates the log files
func (t *Tracer) rotateLogFile() {
	// Close current files
	if t.file != nil {
		t.file.Close()
	}
	if t.htmlFile != nil {
		t.htmlFile.Close()
	}

	// Rename current files with timestamp
	timestamp := time.Now().Format("2006-01-02_15_04_05")
	logFilePath := filepath.Join(FOLDER_NAME, "trace.log")
	htmlFilePath := filepath.Join(FOLDER_NAME, "trace.html")
	newLogFilePath := filepath.Join(FOLDER_NAME, fmt.Sprintf("%s_trace.log", timestamp))
	newHtmlFilePath := filepath.Join(FOLDER_NAME, fmt.Sprintf("%s_trace.html", timestamp))

	if err := os.Rename(logFilePath, newLogFilePath); err != nil {
		log.Printf("Failed to rename log file: %v", err)
	}
	if err := os.Rename(htmlFilePath, newHtmlFilePath); err != nil {
		log.Printf("Failed to rename HTML log file: %v", err)
	}

	// Open new files
	var err error
	t.file, err = os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Printf("Failed to open new log file: %v", err)
		t.file = nil
	}

	t.htmlFile, err = os.OpenFile(htmlFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Printf("Failed to open new HTML log file: %v", err)
		t.htmlFile = nil
	} else {
		if err := t.writeHTMLHeader(); err != nil {
			log.Printf("Failed to write HTML header: %v", err)
		}
	}

	// Reset size
	t.size = 0

	// Update logger
	if t.file != nil {
		mw := io.MultiWriter(os.Stdout, t.file)
		t.logger = log.New(mw, "", log.LstdFlags)
	} else {
		t.logger = log.New(os.Stdout, "", log.LstdFlags)
	}

	// Clean up old log files
	t.cleanOldLogFiles()
}

// cleanOldLogFiles removes the oldest log files if there are more than MAX_FILES
func (t *Tracer) cleanOldLogFiles() {
	// Get all log files
	logFiles, err := filepath.Glob(filepath.Join(FOLDER_NAME, "*_trace.log"))
	if err != nil {
		log.Printf("Failed to glob log files: %v", err)
		return
	}

	htmlFiles, err := filepath.Glob(filepath.Join(FOLDER_NAME, "*_trace.html"))
	if err != nil {
		log.Printf("Failed to glob HTML log files: %v", err)
		return
	}

	// Sort log files by creation time
	type fileInfo struct {
		path    string
		modTime time.Time
	}

	var allFiles []fileInfo
	for _, path := range append(logFiles, htmlFiles...) {
		info, err := os.Stat(path)
		if err != nil {
			log.Printf("Failed to stat file %s: %v", path, err)
			continue
		}
		allFiles = append(allFiles, fileInfo{path, info.ModTime()})
	}

	// Sort by modification time (oldest first)
	sort.Slice(allFiles, func(i, j int) bool {
		return allFiles[i].modTime.Before(allFiles[j].modTime)
	})

	// Remove oldest files if we have more than MAX_FILES
	if len(allFiles) > MAX_FILES {
		for _, file := range allFiles[:len(allFiles)-MAX_FILES] {
			if err := os.Remove(file.path); err != nil {
				log.Printf("Failed to remove old log file %s: %v", file.path, err)
			}
		}
	}
}

// writeHTMLHeader writes the HTML header to the HTML log file
func (t *Tracer) writeHTMLHeader() error {
	header := `<!DOCTYPE html>
<meta content="text/html;charset=utf-8" http-equiv="Content-Type">
<script>
var original_html = null;
var filter = '';
function filter_log()
{
    document.body.style.cursor = 'wait';
    if (original_html == null) {
        original_html = document.body.innerHTML;
    }
    if (filter == '') {
        document.body.innerHTML = original_html;
    } else {
        l = original_html.split("<br>");
        var pattern = new RegExp(".*" + filter.replace('"', '\\"') + ".*", "i");
        final_html = '';
        for(var i=0; i<l.length; i++){
            if (pattern.test(l[i]))
                final_html += l[i] + '<br>';
        }
        document.body.innerHTML = final_html;
    }
    document.body.style.cursor = 'default';
}

document.onkeydown = function(event) {
    if (event.keyCode == 76) {
        var ret = prompt("Enter the filter regular expression. Examples:\\n\\n\\
CheckFirmwareUpdate'\\n\\n'ID=1 |ID=2 \\n\\nID=2 .*Got message\\n\\n2012-08-31 16:.*(ID=1 |ID=2 )\\n\\n", filter);
        if (ret != null) {
            filter = ret;
            filter_log();
        }
        return false;
    }
}
</script>
<STYLE TYPE="text/css">
<!--
BODY
{
  color:white;
  background-color:black;
  font-family:monospace, sans-serif;
}
-->
</STYLE>
<body bgcolor="black" text="white">
<font color="white">
`
	_, err := t.htmlFile.WriteString(header)
	return err
}

// Helper functions

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
