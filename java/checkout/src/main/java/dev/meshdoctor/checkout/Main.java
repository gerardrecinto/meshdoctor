package dev.meshdoctor.checkout;

import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpServer;
import java.io.IOException;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.Executors;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.function.DoubleSupplier;

/**
 * Small HTTP service used as the mesh workload. VERSION is what the canary test reads back.
 * FAIL_RATE (0 to 1) makes /checkout return 500 for that share of calls, so outlier
 * detection has something to eject. SIGTERM flips /readyz to 503 first, then drains.
 */
public final class Main {
    private final String version;
    private final double failRate;
    private final DoubleSupplier random;
    private final AtomicBoolean draining = new AtomicBoolean();

    Main(String version, double failRate, DoubleSupplier random) {
        this.version = version;
        this.failRate = failRate;
        this.random = random;
    }

    HttpServer start(int port) throws IOException {
        HttpServer s = HttpServer.create(new InetSocketAddress(port), 0);
        s.createContext("/healthz", x -> reply(x, 200, "ok"));
        s.createContext("/readyz", x -> {
            if (draining.get()) {
                reply(x, 503, "draining");
            } else {
                reply(x, 200, "ready");
            }
        });
        s.createContext("/checkout", this::checkout);
        s.setExecutor(Executors.newFixedThreadPool(8));
        s.start();
        return s;
    }

    private void checkout(HttpExchange x) throws IOException {
        if (failRate > 0 && random.getAsDouble() < failRate) {
            reply(x, 500, "error " + version);
        } else {
            reply(x, 200, version);
        }
    }

    private static void reply(HttpExchange x, int code, String body) throws IOException {
        byte[] b = body.getBytes(StandardCharsets.UTF_8);
        x.sendResponseHeaders(code, b.length);
        try (var out = x.getResponseBody()) {
            out.write(b);
        }
    }

    static double parseRate(String s) {
        if (s == null || s.isBlank()) {
            return 0;
        }
        double r = Double.parseDouble(s);
        if (r < 0 || r > 1) {
            throw new IllegalArgumentException("FAIL_RATE must be between 0 and 1, got " + s);
        }
        return r;
    }

    public static void main(String[] args) throws Exception {
        String v = System.getenv().getOrDefault("VERSION", "v1");
        int port = Integer.parseInt(System.getenv().getOrDefault("PORT", "8080"));
        Main app = new Main(v, parseRate(System.getenv("FAIL_RATE")), Math::random);
        HttpServer server = app.start(port);
        Runtime.getRuntime().addShutdownHook(new Thread(() -> {
            app.draining.set(true);
            try {
                Thread.sleep(3000);
            } catch (InterruptedException ignored) {
                Thread.currentThread().interrupt();
            }
            server.stop(2);
        }));
        System.out.println("checkout " + v + " listening on " + port);
    }
}
