package com.kentgram;

import android.content.Context;
import android.content.SharedPreferences;
import android.os.Handler;
import android.os.Looper;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.IOException;
import java.util.concurrent.TimeUnit;

import okhttp3.Call;
import okhttp3.HttpUrl;
import okhttp3.MediaType;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;
import okhttp3.WebSocket;
import okhttp3.WebSocketListener;

public final class RealtimeClient {
    public interface Listener {
        void onEvents(JSONArray events);
        void onAuthExpired();
    }

    private static final String TICKET_URL =
            "https://api.kentgram.tendercheck.pro/api/v1/realtime/ticket";
    private static final String SOCKET_URL =
            "https://api.kentgram.tendercheck.pro/api/v1/realtime";
    private static final String PREF_CURSOR = "realtime_cursor";

    private final Handler handler = new Handler(Looper.getMainLooper());
    private final SharedPreferences prefs;
    private final String accessToken;
    private final Listener listener;
    private final OkHttpClient client;
    private volatile boolean stopped;
    private boolean ticketInFlight;
    private int reconnectAttempt;
    private Call ticketCall;
    private WebSocket socket;
    private Runnable reconnectTask;

    public RealtimeClient(Context context, String accessToken, Listener listener) {
        this.prefs = context.getApplicationContext()
                .getSharedPreferences("kentgram", Context.MODE_PRIVATE);
        this.accessToken = accessToken;
        this.listener = listener;
        this.client = new OkHttpClient.Builder()
                .connectTimeout(10, TimeUnit.SECONDS)
                .readTimeout(0, TimeUnit.MILLISECONDS)
                .pingInterval(20, TimeUnit.SECONDS)
                .build();
    }

    public void start() {
        handler.post(this::connect);
    }

    public void stop() {
        stopped = true;
        handler.removeCallbacksAndMessages(null);
        if (ticketCall != null) ticketCall.cancel();
        if (socket != null) {
            socket.close(1000, "client stopped");
            socket = null;
        }
        client.dispatcher().cancelAll();
    }

    private void connect() {
        if (stopped || ticketInFlight || socket != null) return;
        ticketInFlight = true;

        RequestBody body = RequestBody.create(
                "", MediaType.parse("application/json; charset=utf-8"));
        Request request = new Request.Builder()
                .url(TICKET_URL)
                .post(body)
                .header("Authorization", "Bearer " + accessToken)
                .header("Accept", "application/json")
                .build();

        ticketCall = client.newCall(request);
        ticketCall.enqueue(new okhttp3.Callback() {
            @Override public void onFailure(Call call, IOException e) {
                handler.post(() -> {
                    ticketInFlight = false;
                    if (!stopped) scheduleReconnect();
                });
            }

            @Override public void onResponse(Call call, Response response) {
                String raw = "";
                int status = response.code();
                try {
                    if (response.body() != null) raw = response.body().string();
                } catch (IOException ignored) {
                } finally {
                    response.close();
                }
                final String payload = raw;
                handler.post(() -> {
                    ticketInFlight = false;
                    if (stopped) return;
                    if (status == 401) {
                        listener.onAuthExpired();
                        return;
                    }
                    if (status < 200 || status >= 300) {
                        scheduleReconnect();
                        return;
                    }
                    try {
                        String ticket = new JSONObject(payload).optString("ticket", "");
                        if (ticket.isEmpty()) {
                            scheduleReconnect();
                            return;
                        }
                        openSocket(ticket);
                    } catch (Exception e) {
                        scheduleReconnect();
                    }
                });
            }
        });
    }

    private void openSocket(String ticket) {
        long cursor = prefs.getLong(PREF_CURSOR, 0L);
        HttpUrl base = HttpUrl.get(SOCKET_URL);
        HttpUrl socketUrl = base.newBuilder()
                .scheme("wss")
                .addQueryParameter("ticket", ticket)
                .addQueryParameter("since", Long.toString(cursor))
                .build();

        Request request = new Request.Builder().url(socketUrl).build();
        socket = client.newWebSocket(request, new WebSocketListener() {
            @Override public void onOpen(WebSocket webSocket, Response response) {
                handler.post(() -> {
                    if (stopped) {
                        webSocket.close(1000, "client stopped");
                        return;
                    }
                    socket = webSocket;
                    reconnectAttempt = 0;
                });
            }

            @Override public void onMessage(WebSocket webSocket, String text) {
                try {
                    JSONObject message = new JSONObject(text);
                    if (!"sync.batch".equals(message.optString("type"))) return;
                    long next = message.optLong("cursor", -1);
                    if (next >= 0) prefs.edit().putLong(PREF_CURSOR, next).apply();
                    JSONArray events = message.optJSONArray("events");
                    if (events != null && events.length() > 0) {
                        handler.post(() -> {
                            if (!stopped && listener != null) listener.onEvents(events);
                        });
                    }
                } catch (Exception ignored) {
                }
            }

            @Override public void onClosed(WebSocket webSocket, int code, String reason) {
                handler.post(() -> {
                    if (socket == webSocket) socket = null;
                    if (stopped) return;
                    if (code == 1008) {
                        listener.onAuthExpired();
                    } else {
                        scheduleReconnect();
                    }
                });
            }

            @Override public void onFailure(WebSocket webSocket, Throwable error, Response response) {
                int status = response == null ? 0 : response.code();
                handler.post(() -> {
                    if (socket == webSocket) socket = null;
                    if (stopped) return;
                    if (status == 401 || status == 403) {
                        listener.onAuthExpired();
                    } else {
                        scheduleReconnect();
                    }
                });
            }
        });
    }

    private void scheduleReconnect() {
        if (stopped || reconnectTask != null) return;
        long delay = Math.min(30_000L, 1_000L << Math.min(reconnectAttempt, 5));
        reconnectAttempt++;
        reconnectTask = () -> {
            reconnectTask = null;
            connect();
        };
        handler.postDelayed(reconnectTask, delay);
    }
}
