package com.kentgram;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.SecureRandom;
import java.util.concurrent.ThreadLocalRandom;
import javax.net.ssl.SSLParameters;
import javax.net.ssl.SSLSocket;
import javax.net.ssl.SSLSocketFactory;

/**
 * Minimal RFC 6455 client for kentGram's /api/v1/ws channel. Pure Java (no
 * Android classes, no java.util.Base64 which needs API 26) so it runs on
 * minSdk 24 and can be tested on a plain JVM.
 *
 * Protocol: connect, send {"type":"auth","token":...} as the first frame, then
 * receive JSON text frames. Server pings are answered; a silent socket is
 * dropped after READ_TIMEOUT_MS and reconnected with exponential backoff.
 * Events are delivered on the client's own thread: post to the UI thread.
 *
 * The Activity owns the lifecycle: start() in onResume, stop() in onPause.
 */
final class RealtimeClient {
    interface TokenSource { String get(); }
    interface Listener { void onEvent(String json); }

    private static final String GUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11";
    private static final int CONNECT_TIMEOUT_MS = 12000;
    private static final int READ_TIMEOUT_MS = 70000; // server pings every 25s
    private static final int MAX_MESSAGE = 1 << 20;

    private final String url;
    private final TokenSource tokens;
    private final Listener listener;
    private final SecureRandom random = new SecureRandom();
    private final Object writeLock = new Object();

    private int generation; // guarded by this
    private boolean running; // guarded by this
    private volatile Socket socket;
    private volatile OutputStream out;
    private volatile long minDelayMs = 1000; // test hook: scales the backoff

    RealtimeClient(String url, TokenSource tokens, Listener listener) {
        this.url = url;
        this.tokens = tokens;
        this.listener = listener;
    }

    /** Test hook: base backoff delay. */
    void setBackoffBaseMs(long ms) { minDelayMs = Math.max(10, ms); }

    synchronized void start() {
        if (running) return;
        running = true;
        final int gen = ++generation;
        Thread t = new Thread(() -> loop(gen), "kentgram-realtime");
        t.setDaemon(true);
        t.start();
    }

    /** Idempotent. Closes the socket so the reader thread unblocks and exits. */
    void stop() {
        synchronized (this) {
            running = false;
            generation++;
        }
        closeSocket();
    }

    private synchronized boolean alive(int gen) { return running && gen == generation; }

    private void loop(int gen) {
        int attempt = 0;
        while (alive(gen)) {
            boolean gotFrame = false;
            try {
                gotFrame = session(gen);
            } catch (Exception ignored) {
                // any failure just leads to a reconnect
            }
            closeSocket();
            if (!alive(gen)) return;
            if (gotFrame) attempt = 0;
            long base = Math.min(30000L, minDelayMs << Math.min(attempt++, 5));
            long delay = base / 2 + ThreadLocalRandom.current().nextLong(base / 2 + 1);
            try {
                Thread.sleep(delay);
            } catch (InterruptedException e) {
                return;
            }
        }
    }

    /** One connection. Returns true if at least one text frame arrived. */
    private boolean session(int gen) throws Exception {
        String token = tokens.get();
        if (token == null || token.isEmpty()) {
            throw new IOException("no token");
        }
        URI uri = URI.create(url);
        boolean tls = "wss".equalsIgnoreCase(uri.getScheme());
        if (!tls && !"ws".equalsIgnoreCase(uri.getScheme())) throw new IOException("bad scheme");
        String host = uri.getHost();
        int port = uri.getPort() > 0 ? uri.getPort() : (tls ? 443 : 80);
        String path = uri.getRawPath() == null || uri.getRawPath().isEmpty() ? "/" : uri.getRawPath();

        Socket plain = new Socket();
        plain.connect(new InetSocketAddress(host, port), CONNECT_TIMEOUT_MS);
        Socket s = plain;
        if (tls) {
            SSLSocket ssl = (SSLSocket) ((SSLSocketFactory) SSLSocketFactory.getDefault()).createSocket(plain, host, port, true);
            // SSLSocket does not verify the host name on its own: require HTTPS-style verification.
            SSLParameters params = ssl.getSSLParameters();
            params.setEndpointIdentificationAlgorithm("HTTPS");
            ssl.setSSLParameters(params);
            ssl.setSoTimeout(CONNECT_TIMEOUT_MS);
            ssl.startHandshake();
            s = ssl;
        }
        s.setTcpNoDelay(true);
        s.setSoTimeout(CONNECT_TIMEOUT_MS);
        socket = s;
        if (!alive(gen)) return false;

        InputStream in = s.getInputStream();
        out = s.getOutputStream();

        byte[] keyRaw = new byte[16];
        random.nextBytes(keyRaw);
        String key = base64(keyRaw);
        String hostHeader = uri.getPort() > 0 ? host + ":" + uri.getPort() : host;
        String req = "GET " + path + " HTTP/1.1\r\nHost: " + hostHeader + "\r\nUpgrade: websocket\r\n"
                + "Connection: Upgrade\r\nSec-WebSocket-Key: " + key + "\r\nSec-WebSocket-Version: 13\r\n\r\n";
        synchronized (writeLock) {
            out.write(req.getBytes(StandardCharsets.US_ASCII));
            out.flush();
        }
        verifyHandshake(in, key);

        sendFrame(0x1, ("{\"type\":\"auth\",\"token\":\"" + jsonEscape(token) + "\"}").getBytes(StandardCharsets.UTF_8));
        s.setSoTimeout(READ_TIMEOUT_MS);
        return readLoop(in, gen);
    }

    private void verifyHandshake(InputStream in, String key) throws Exception {
        ByteArrayOutputStream head = new ByteArrayOutputStream();
        int state = 0; // matching \r\n\r\n
        while (state < 4) {
            int b = in.read();
            if (b < 0) throw new IOException("closed during handshake");
            head.write(b);
            if (head.size() > 8192) throw new IOException("handshake too large");
            if ((b == '\r' && (state == 0 || state == 2)) || (b == '\n' && (state == 1 || state == 3))) state++;
            else state = b == '\r' ? 1 : 0;
        }
        String text = new String(head.toByteArray(), StandardCharsets.ISO_8859_1);
        String[] lines = text.split("\r\n");
        if (lines.length == 0 || !lines[0].startsWith("HTTP/1.1 101")) throw new IOException("upgrade refused: " + lines[0]);
        MessageDigest sha1 = MessageDigest.getInstance("SHA-1");
        String expected = base64(sha1.digest((key + GUID).getBytes(StandardCharsets.US_ASCII)));
        String accept = null;
        for (String line : lines) {
            int i = line.indexOf(':');
            if (i > 0 && line.substring(0, i).trim().equalsIgnoreCase("Sec-WebSocket-Accept")) accept = line.substring(i + 1).trim();
        }
        if (!expected.equals(accept)) throw new IOException("bad Sec-WebSocket-Accept");
    }

    private boolean readLoop(InputStream in, int gen) throws IOException {
        boolean gotFrame = false;
        ByteArrayOutputStream message = new ByteArrayOutputStream();
        boolean inText = false;
        while (alive(gen)) {
            int b0 = in.read();
            if (b0 < 0) return gotFrame;
            int b1 = in.read();
            if (b1 < 0) return gotFrame;
            boolean fin = (b0 & 0x80) != 0;
            int op = b0 & 0x0F;
            boolean masked = (b1 & 0x80) != 0;
            long len = b1 & 0x7F;
            if (len == 126) len = ((long) readByte(in) << 8) | readByte(in);
            else if (len == 127) {
                len = 0;
                for (int i = 0; i < 8; i++) len = (len << 8) | readByte(in);
            }
            if (len < 0 || len > MAX_MESSAGE) throw new IOException("frame too large");
            byte[] mask = null;
            if (masked) {
                mask = new byte[4];
                readFully(in, mask, 4);
            }
            byte[] payload = new byte[(int) len];
            readFully(in, payload, payload.length);
            if (mask != null) for (int i = 0; i < payload.length; i++) payload[i] ^= mask[i & 3];

            switch (op) {
                case 0x9: // ping
                    sendFrame(0xA, payload);
                    break;
                case 0xA: // pong
                    break;
                case 0x8: // close
                    try { sendFrame(0x8, payload); } catch (IOException ignored) { }
                    return gotFrame;
                case 0x1:
                case 0x0:
                    if (op == 0x1) { message.reset(); inText = true; }
                    else if (!inText) throw new IOException("unexpected continuation");
                    message.write(payload);
                    if (message.size() > MAX_MESSAGE) throw new IOException("message too large");
                    if (fin) {
                        inText = false;
                        gotFrame = true;
                        String json = new String(message.toByteArray(), StandardCharsets.UTF_8);
                        message.reset();
                        if (alive(gen)) {
                            try { listener.onEvent(json); } catch (RuntimeException ignored) { }
                        }
                    }
                    break;
                default:
                    break; // binary / reserved: ignore
            }
        }
        return gotFrame;
    }

    private static int readByte(InputStream in) throws IOException {
        int b = in.read();
        if (b < 0) throw new IOException("eof");
        return b;
    }

    private static void readFully(InputStream in, byte[] buf, int n) throws IOException {
        int off = 0;
        while (off < n) {
            int r = in.read(buf, off, n - off);
            if (r < 0) throw new IOException("eof");
            off += r;
        }
    }

    /** Client frames must be masked (RFC 6455 5.3). */
    private void sendFrame(int op, byte[] payload) throws IOException {
        OutputStream o = out;
        if (o == null) throw new IOException("not connected");
        byte[] mask = new byte[4];
        random.nextBytes(mask);
        ByteArrayOutputStream f = new ByteArrayOutputStream(payload.length + 14);
        f.write(0x80 | op);
        int n = payload.length;
        if (n < 126) f.write(0x80 | n);
        else if (n <= 0xFFFF) { f.write(0x80 | 126); f.write(n >> 8); f.write(n); }
        else { f.write(0x80 | 127); for (int i = 7; i >= 0; i--) f.write((int) (((long) n >> (8 * i)) & 0xFF)); }
        f.write(mask);
        for (int i = 0; i < n; i++) f.write(payload[i] ^ mask[i & 3]);
        synchronized (writeLock) {
            o.write(f.toByteArray());
            o.flush();
        }
    }

    private void closeSocket() {
        Socket s = socket;
        socket = null;
        out = null;
        if (s != null) {
            try { s.close(); } catch (IOException ignored) { }
        }
    }

    private static String jsonEscape(String v) {
        StringBuilder b = new StringBuilder(v.length() + 8);
        for (int i = 0; i < v.length(); i++) {
            char c = v.charAt(i);
            if (c == '"' || c == '\\') b.append('\\').append(c);
            else if (c < 0x20) b.append(String.format("\\u%04x", (int) c));
            else b.append(c);
        }
        return b.toString();
    }

    private static final char[] B64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/".toCharArray();

    static String base64(byte[] d) {
        StringBuilder sb = new StringBuilder((d.length + 2) / 3 * 4);
        for (int i = 0; i < d.length; i += 3) {
            int v = (d[i] & 0xFF) << 16 | (i + 1 < d.length ? (d[i + 1] & 0xFF) << 8 : 0) | (i + 2 < d.length ? d[i + 2] & 0xFF : 0);
            sb.append(B64[v >> 18 & 63]).append(B64[v >> 12 & 63]);
            sb.append(i + 1 < d.length ? B64[v >> 6 & 63] : '=').append(i + 2 < d.length ? B64[v & 63] : '=');
        }
        return sb.toString();
    }
}
