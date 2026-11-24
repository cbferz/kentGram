package com.kentgram;

import android.content.Context;
import android.content.SharedPreferences;
import org.json.JSONException;
import org.json.JSONObject;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;

/**
 * Process-wide owner of the signed-in session, shared by every activity.
 *
 * The server issues 15-minute access tokens and rotates the refresh token on
 * every use; presenting an already rotated refresh token is treated as theft
 * and revokes the whole session. So:
 *  - at most one refresh runs at a time in this process, and a caller that
 *    waited for it reuses its result instead of rotating again;
 *  - the rotated pair is committed to disk before anyone may use it;
 *  - only an explicit rejection of the refresh token ends the local session.
 *    A single 401, a timeout or a server error never signs the user out.
 */
final class Session {
    static final String API="https://api.kentgram.tendercheck.pro/api/v1";
    static final String PREFS="kentgram";

    private static final String KEY_ACCESS="access_token";
    private static final String KEY_REFRESH="refresh_token";
    private static final String KEY_EXPIRES_AT="access_expires_at";
    /** Refresh proactively this long before the access token expires. */
    private static final long REFRESH_AHEAD_MS=60_000L;

    /** Serialises refresh requests: refresh tokens are single-use. */
    private static final Object REFRESH_LOCK=new Object();
    /** Guards token writes and {@link #generation}; never held across network I/O. */
    private static final Object STATE_LOCK=new Object();
    /** Bumped on every sign-in and sign-out so stale refreshes cannot resurrect tokens. */
    private static int generation;

    enum Refresh { REFRESHED, REJECTED, UNAVAILABLE }

    /** Writes a request body. Called again when a request is retried after a refresh. */
    interface BodyWriter { void write(HttpURLConnection connection) throws IOException; }

    static final class Result {
        final int status;
        final JSONObject body;
        /** The server rejected the refresh token: the user has to sign in again. */
        final boolean sessionLost;

        private Result(int status,JSONObject body,boolean sessionLost){
            this.status=status;
            this.body=body;
            this.sessionLost=sessionLost;
        }

        boolean ok(){return status>=200&&status<300;}

        static Result offline(){return new Result(0,error("connection_failed"),false);}
        static Result lost(){return new Result(401,error("session_expired"),true);}
    }

    private Session(){}

    private static SharedPreferences prefs(Context context){
        return context.getApplicationContext().getSharedPreferences(PREFS,Context.MODE_PRIVATE);
    }

    static String accessToken(Context context){return prefs(context).getString(KEY_ACCESS,"");}

    static String refreshToken(Context context){return prefs(context).getString(KEY_REFRESH,"");}

    static boolean isSignedIn(Context context){
        SharedPreferences prefs=prefs(context);
        return !prefs.getString(KEY_ACCESS,"").isEmpty()||!prefs.getString(KEY_REFRESH,"").isEmpty();
    }

    static int generation(){
        synchronized(STATE_LOCK){return generation;}
    }

    /** Stores the tokens of a fresh login or registration. */
    static boolean signIn(Context context,JSONObject response){
        String access=response.optString("access_token","");
        if(access.isEmpty())return false;
        synchronized(STATE_LOCK){
            generation++;
            write(context,access,response.optString("refresh_token",""),response.optLong("expires_in",0));
        }
        return true;
    }

    /** Forgets the local session and everything cached alongside it. */
    static void clear(Context context){
        synchronized(STATE_LOCK){
            generation++;
            prefs(context).edit().clear().commit();
        }
    }

    private static void write(Context context,String access,String refresh,long expiresInSeconds){
        long expiresAt=expiresInSeconds>0?System.currentTimeMillis()+expiresInSeconds*1000L:0L;
        // commit(), not apply(): the previous refresh token is already invalid on
        // the server, so the new pair must be on disk before it is used.
        prefs(context).edit()
                .putString(KEY_ACCESS,access)
                .putString(KEY_REFRESH,refresh)
                .putLong(KEY_EXPIRES_AT,expiresAt)
                .commit();
    }

    private static boolean expiresSoon(Context context){
        long expiresAt=prefs(context).getLong(KEY_EXPIRES_AT,0L);
        return expiresAt>0&&System.currentTimeMillis()>=expiresAt-REFRESH_AHEAD_MS;
    }

    /**
     * Returns an access token that is not about to expire, refreshing it first
     * when needed. Blocks on network I/O: never call on the main thread.
     */
    static String usableAccessToken(Context context){
        String access=accessToken(context);
        if(!access.isEmpty()&&expiresSoon(context))refresh(context,access,generation());
        return accessToken(context);
    }

    /**
     * Replaces {@code staleAccess} with a new access token. If another caller
     * already replaced it while this one waited, nothing is sent and the new
     * token is reused. Blocks on network I/O: never call on the main thread.
     *
     * @param startGeneration {@link #generation()} observed before the request
     *                        that failed; a sign-in or sign-out since then makes
     *                        the refresh moot.
     */
    static Refresh refresh(Context context,String staleAccess,int startGeneration){
        synchronized(REFRESH_LOCK){
            if(generation()!=startGeneration)return Refresh.UNAVAILABLE;
            String current=accessToken(context);
            if(!current.isEmpty()&&!current.equals(staleAccess))return Refresh.REFRESHED;
            String refreshToken=refreshToken(context);
            if(refreshToken.isEmpty())return Refresh.REJECTED;

            JSONObject body=new JSONObject();
            try{body.put("refresh_token",refreshToken);}catch(JSONException ignored){}
            Result result=send("POST","/auth/refresh",json(body),"",12000,15000);
            String nextAccess=result.body.optString("access_token","");
            if(result.ok()&&!nextAccess.isEmpty()){
                synchronized(STATE_LOCK){
                    if(generation!=startGeneration)return Refresh.UNAVAILABLE;
                    write(context,nextAccess,result.body.optString("refresh_token",refreshToken),
                            result.body.optLong("expires_in",0));
                }
                return Refresh.REFRESHED;
            }
            // 401: the refresh token is unknown, expired, revoked or was replayed.
            if(result.status==401||result.status==403)
                return generation()==startGeneration?Refresh.REJECTED:Refresh.UNAVAILABLE;
            // Offline, timeout, 429 or 5xx: keep the session and try again later.
            return Refresh.UNAVAILABLE;
        }
    }

    /**
     * Performs an API request. With {@code auth}, an expired access token is
     * refreshed and the request is retried once. Blocks on network I/O.
     */
    static Result call(Context context,String method,String path,BodyWriter body,boolean auth,
                       int connectTimeoutMs,int readTimeoutMs){
        if(!auth)return send(method,path,body,"",connectTimeoutMs,readTimeoutMs);
        int startGeneration=generation();
        String access=accessToken(context);
        if(!access.isEmpty()&&expiresSoon(context)){
            if(refresh(context,access,startGeneration)==Refresh.REJECTED)return Result.lost();
            access=accessToken(context);
        }
        Result first=send(method,path,body,access,connectTimeoutMs,readTimeoutMs);
        if(first.status!=401)return first;
        switch(refresh(context,access,startGeneration)){
            case REFRESHED:
                return send(method,path,body,accessToken(context),connectTimeoutMs,readTimeoutMs);
            case REJECTED:
                return Result.lost();
            default:
                return Result.offline();
        }
    }

    static Result call(Context context,String method,String path,JSONObject body,boolean auth){
        return call(context,method,path,json(body),auth,12000,15000);
    }

    /**
     * Revokes a session on the server after it was cleared locally. Best effort:
     * the user is already signed out on this device either way.
     */
    static void revoke(String access,String refresh){
        if(access.isEmpty()&&refresh.isEmpty())return;
        JSONObject empty=new JSONObject();
        Result result=access.isEmpty()?Result.lost():
                send("POST","/auth/logout",json(empty),access,10000,10000);
        if(result.status!=401||refresh.isEmpty())return;
        // The access token had expired: trade the refresh token for one last
        // access token so the session can still be revoked.
        JSONObject body=new JSONObject();
        try{body.put("refresh_token",refresh);}catch(JSONException ignored){}
        Result renewed=send("POST","/auth/refresh",json(body),"",10000,10000);
        String nextAccess=renewed.body.optString("access_token","");
        if(renewed.ok()&&!nextAccess.isEmpty())
            send("POST","/auth/logout",json(empty),nextAccess,10000,10000);
    }

    static BodyWriter json(JSONObject body){
        if(body==null)return null;
        byte[] bytes=body.toString().getBytes(StandardCharsets.UTF_8);
        return connection->{
            connection.setRequestProperty("Content-Type","application/json");
            connection.setFixedLengthStreamingMode(bytes.length);
            try(OutputStream output=connection.getOutputStream()){output.write(bytes);}
        };
    }

    private static Result send(String method,String path,BodyWriter body,String access,
                               int connectTimeoutMs,int readTimeoutMs){
        HttpURLConnection connection=null;
        try{
            connection=(HttpURLConnection)new URL(path.startsWith("https://")?path:API+path).openConnection();
            connection.setRequestMethod(method);
            connection.setConnectTimeout(connectTimeoutMs);
            connection.setReadTimeout(readTimeoutMs);
            connection.setRequestProperty("Accept","application/json");
            if(!access.isEmpty())connection.setRequestProperty("Authorization","Bearer "+access);
            if(body!=null){
                connection.setDoOutput(true);
                body.write(connection);
            }
            int status=connection.getResponseCode();
            InputStream input=status>=200&&status<300?connection.getInputStream():connection.getErrorStream();
            JSONObject json=new JSONObject();
            String text=input==null?"":readText(input);
            if(!text.trim().isEmpty()){
                try{
                    json=new JSONObject(text);
                }catch(JSONException parseError){
                    if(status<200||status>=300)
                        json=error(status==404?"endpoint_not_found":status>=500?"server_error":"http_error");
                }
            }else if(status<200||status>=300){
                json=error(status>=500?"server_error":"http_error");
            }
            return new Result(status,json,false);
        }catch(IOException|RuntimeException failure){
            return Result.offline();
        }finally{
            if(connection!=null)connection.disconnect();
        }
    }

    private static String readText(InputStream input)throws IOException{
        try(InputStream stream=input;ByteArrayOutputStream bytes=new ByteArrayOutputStream()){
            byte[] buffer=new byte[4096];
            int count;
            while((count=stream.read(buffer))!=-1)bytes.write(buffer,0,count);
            return bytes.toString("UTF-8");
        }
    }

    private static JSONObject error(String code){
        JSONObject json=new JSONObject();
        try{json.put("error",code);}catch(JSONException ignored){}
        return json;
    }
}
