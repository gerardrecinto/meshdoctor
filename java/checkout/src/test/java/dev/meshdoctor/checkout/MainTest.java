package dev.meshdoctor.checkout;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

import com.sun.net.httpserver.HttpServer;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import org.junit.jupiter.api.Test;

class MainTest {
    private static HttpResponse<String> get(HttpServer s, String path) throws Exception {
        var req = HttpRequest.newBuilder(URI.create("http://127.0.0.1:" + s.getAddress().getPort() + path)).build();
        return HttpClient.newHttpClient().send(req, HttpResponse.BodyHandlers.ofString());
    }

    @Test
    void returnsItsVersion() throws Exception {
        HttpServer s = new Main("v2", 0, () -> 0.5).start(0);
        try {
            var r = get(s, "/checkout");
            assertEquals(200, r.statusCode());
            assertEquals("v2", r.body());
            assertEquals(200, get(s, "/healthz").statusCode());
            assertEquals(200, get(s, "/readyz").statusCode());
        } finally {
            s.stop(0);
        }
    }

    @Test
    void failsForTheConfiguredShare() throws Exception {
        HttpServer always = new Main("v2", 1.0, () -> 0.0).start(0);
        HttpServer never = new Main("v2", 0.3, () -> 0.9).start(0);
        try {
            assertEquals(500, get(always, "/checkout").statusCode());
            assertEquals(200, get(never, "/checkout").statusCode());
            assertEquals(200, get(always, "/healthz").statusCode());
        } finally {
            always.stop(0);
            never.stop(0);
        }
    }

    @Test
    void parsesFailRate() {
        assertEquals(0.0, Main.parseRate(null));
        assertEquals(0.25, Main.parseRate("0.25"));
        assertThrows(IllegalArgumentException.class, () -> Main.parseRate("1.5"));
    }
}
