package demo.reader;

import java.nio.file.Path;

public record Reader(Path path) {
    public String title() {
        return "Lumeleaf — " + path.getFileName();
    }

    public boolean isMarkdown() {
        return path.toString().endsWith(".md");
    }
}
