package dev.foxxycode.intellij.hooks;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URI;

/**
 * A dependency-free process entry point for Coddy's command hooks. The agent's JSON payload goes
 * to the IDE unchanged; the IDE's JSON answer goes to stdout unchanged. An unavailable IDE declines
 * the hook without output, which leaves ordinary local execution in place.
 */
public final class FoxxyCodeHookProcess {
    private FoxxyCodeHookProcess() {}

    public static void main(String[] args) {
        if (args.length < 2) return;
        HttpURLConnection connection = null;
        try {
            byte[] payload = readAll(System.in);
            connection = (HttpURLConnection) URI.create(args[0]).toURL().openConnection();
            connection.setRequestMethod("POST");
            connection.setConnectTimeout(3000);
            connection.setReadTimeout(58000);
            connection.setDoOutput(true);
            connection.setRequestProperty("Content-Type", "application/json");
            connection.setRequestProperty("X-FoxxyCode-Token", args[1]);
            connection.setFixedLengthStreamingMode(payload.length);
            try (OutputStream out = connection.getOutputStream()) {
                out.write(payload);
            }
            if (connection.getResponseCode() != 200) return;
            try (InputStream in = connection.getInputStream()) {
                in.transferTo(System.out);
            }
        } catch (Exception ignored) {
            // A stopped IDE must not turn a hook transport failure into a tool denial.
        } finally {
            if (connection != null) connection.disconnect();
        }
    }

    private static byte[] readAll(InputStream in) throws IOException {
        return in.readAllBytes();
    }
}
