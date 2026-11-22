package com.kentgram;

import android.app.Activity;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.graphics.Color;
import android.graphics.Typeface;
import android.os.Bundle;
import android.view.Gravity;
import android.widget.*;
import android.content.SharedPreferences;
import org.json.JSONObject;
import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public final class ProfileActivity extends Activity {
    private static final String API="https://api.kentgram.tendercheck.pro/api/v1";
    private static final int BG=0xFF090B10,PANEL=0xFF121720,TEXT=0xFFF4F6FA,MUTED=0xFF8994A6,BLUE=0xFF648DFF;
    private final ExecutorService io=Executors.newFixedThreadPool(2);
    private String token="", username="";
    private boolean own, following;
    private long userId;
    private LinearLayout content;
    private ImageView avatar;
    private EditText displayName,bio,avatarUrl;
    private Button followButton,saveButton;

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        getWindow().setStatusBarColor(Color.BLACK);
        getWindow().setNavigationBarColor(BG);
        SharedPreferences prefs=getSharedPreferences("kentgram",MODE_PRIVATE);
        token=prefs.getString("access_token","");
        username=getIntent().getStringExtra("username");
        own=username==null || username.isEmpty();
        build();
        load();
    }

    private void build() {
        LinearLayout root=new LinearLayout(this);
        root.setOrientation(1);
        root.setBackgroundColor(BG);

        LinearLayout header=new LinearLayout(this);
        header.setGravity(Gravity.CENTER_VERTICAL);
        header.setPadding(dp(18),dp(12),dp(18),dp(12));
        Button back=button("‹");
        back.setTextSize(25);
        back.setBackground(round(0xFF1A2230,18));
        header.addView(back,new LinearLayout.LayoutParams(dp(48),dp(46)));
        back.setOnClickListener(v->finish());
        TextView title=label(own?"Профиль":"Профиль пользователя",19,TEXT,true);
        LinearLayout.LayoutParams tp=new LinearLayout.LayoutParams(0,-2,1);
        tp.leftMargin=dp(12);
        header.addView(title,tp);
        root.addView(header);

        ScrollView scroll=new ScrollView(this);
        scroll.setClipToPadding(false);
        content=new LinearLayout(this);
        content.setOrientation(1);
        content.setPadding(dp(18),dp(18),dp(18),dp(28));
        scroll.addView(content);
        root.addView(scroll,new LinearLayout.LayoutParams(-1,0,1));
        setContentView(root);

        avatar=new ImageView(this);
        avatar.setImageDrawable(getDrawable(android.R.drawable.sym_def_app_icon));
        avatar.setScaleType(ImageView.ScaleType.CENTER_CROP);
        avatar.setBackground(round(0xFF202D43,100));
        avatar.setClipToOutline(true);
        avatar.setElevation(dp(5));
        LinearLayout.LayoutParams ap=new LinearLayout.LayoutParams(dp(116),dp(116));
        ap.gravity=Gravity.CENTER;
        ap.bottomMargin=dp(15);
        content.addView(avatar,ap);

        TextView identity=label(own?"Ваш аккаунт":"Профиль kentGram",18,TEXT,true);
        identity.setGravity(Gravity.CENTER);
        LinearLayout.LayoutParams ip=new LinearLayout.LayoutParams(-1,-2);
        ip.bottomMargin=dp(18);
        content.addView(identity,ip);

        displayName=field("Имя");
        bio=field("О себе — до 160 символов");
        avatarUrl=field("HTTPS-ссылка на аватар");
        if(own) {
            LinearLayout card=new LinearLayout(this);
            card.setOrientation(1);
            card.setPadding(dp(16),dp(12),dp(16),dp(14));
            card.setBackground(round(PANEL,22));
            content.addView(card);
            section(card,"Имя пользователя");
            section(card,"Отображаемое имя");
            card.addView(displayName);
            section(card,"О себе");
            card.addView(bio);
            section(card,"Аватар");
            card.addView(avatarUrl);
            TextView hint=label("Пока поддерживается ссылка на изображение HTTPS. Загрузка фото с устройства появится следующим этапом.",12,MUTED,false);
            hint.setPadding(0,0,0,dp(12));
            card.addView(hint);
            saveButton=button("Сохранить изменения");
            card.addView(saveButton);
            saveButton.setOnClickListener(v->save());
        } else {
            LinearLayout card=new LinearLayout(this);
            card.setOrientation(1);
            card.setPadding(dp(16),dp(12),dp(16),dp(14));
            card.setBackground(round(PANEL,22));
            content.addView(card);
            section(card,"Имя пользователя");
            card.addView(label("@"+username,15,MUTED,true));
            section("Имя");
            card.addView(displayName);
            section(card,"О себе");
            card.addView(bio);
            displayName.setEnabled(false);
            bio.setEnabled(false);
            followButton=button("Подписаться");
            LinearLayout.LayoutParams fp=new LinearLayout.LayoutParams(-1,-2);
            fp.topMargin=dp(12);
            card.addView(followButton,fp);
            followButton.setOnClickListener(v->toggleFollow());
        }
        TextView social=label("ПОДПИСЧИКИ И ПОДПИСКИ",10,MUTED,true);
        social.setLetterSpacing(.12f);
        social.setPadding(dp(2),dp(18),0,dp(8));
        content.addView(social);
    }

    private void load() {
        String path=own?"/me":"/users/"+enc(username);
        call("GET",path,null,result->{
            userId=result.optLong("id");String handle=result.optString("username",username==null?"":username);
            displayName.setText(result.optString("display_name",""));
            bio.setText(result.optString("bio",""));
            avatarUrl.setText(result.optString("avatar_url",""));
            if(own) {
                TextView handleView=label("@"+handle,15,MUTED,true);
                content.addView(handleView,2);
                username=handle;
            } else {
                following=result.optBoolean("is_following",false);
                followButton.setText(following?"Вы подписаны":"Подписаться");
            }
            long followers=result.optLong("followers"),followingCount=result.optLong("following");
            content.addView(label(followers+" подписчиков  ·  "+followingCount+" подписок",14,MUTED,false));
            loadAvatar(result.optString("avatar_url",""));
        });
    }

    private void save() {
        JSONObject body=new JSONObject();
        try { body.put("display_name",displayName.getText().toString().trim());body.put("bio",bio.getText().toString().trim());body.put("avatar_url",avatarUrl.getText().toString().trim()); }
        catch(Exception e){toast("Не удалось подготовить данные");return;}
        saveButton.setEnabled(false);
        call("PATCH","/me/profile",body,result->{saveButton.setEnabled(true);toast("Профиль сохранён");loadAvatar(result.optString("avatar_url",""));});
    }

    private void toggleFollow() {
        if(userId<=0)return;
        followButton.setEnabled(false);
        call(following?"DELETE":"POST","/users/"+userId+"/follow",null,result->{
            following=!following;followButton.setText(following?"Вы подписаны":"Подписаться");followButton.setEnabled(true);
        });
    }

    private void loadAvatar(String url) {
        if(url==null||url.isEmpty())return;
        io.execute(()->{
            try {
                HttpURLConnection c=(HttpURLConnection)new URL(url).openConnection();c.setConnectTimeout(8000);c.setReadTimeout(8000);
                c.setInstanceFollowRedirects(false);
                try(InputStream in=c.getInputStream()){Bitmap bitmap=BitmapFactory.decodeStream(in);if(bitmap!=null)runOnUiThread(()->avatar.setImageBitmap(bitmap));}
                c.disconnect();
            } catch(Exception ignored) {}
        });
    }

    private interface Done { void accept(JSONObject value); }
    private void call(String method,String path,JSONObject body,Done done) {
        io.execute(()->{
            JSONObject out=new JSONObject();int status=0;
            try {
                HttpURLConnection c=(HttpURLConnection)new URL(API+path).openConnection();
                c.setRequestMethod(method);c.setConnectTimeout(12000);c.setReadTimeout(12000);
                c.setRequestProperty("Accept","application/json");c.setRequestProperty("Authorization","Bearer "+token);
                if(body!=null){c.setDoOutput(true);c.setRequestProperty("Content-Type","application/json");try(OutputStream s=c.getOutputStream()){s.write(body.toString().getBytes(StandardCharsets.UTF_8));}}
                status=c.getResponseCode();InputStream in=status>=200&&status<300?c.getInputStream():c.getErrorStream();
                if(in!=null){ByteArrayOutputStream b=new ByteArrayOutputStream();byte[] buf=new byte[4096];int n;while((n=in.read(buf))>0)b.write(buf,0,n);out=new JSONObject(b.toString("UTF-8"));}
                c.disconnect();
            } catch(Exception e){try{out.put("error","network_error");}catch(Exception ignored){}}
            final JSONObject result=out;final int code=status;
            runOnUiThread(()->{if(code>=200&&code<300)done.accept(result);else{if(saveButton!=null)saveButton.setEnabled(true);if(followButton!=null)followButton.setEnabled(true);KentDialog.showMessage(this,"Ошибка профиля",result.optString("error","Ошибка соединения")+" ("+code+")","Понятно",null);}});
        });
    }

    private void section(String s){section(content,s);}
    private void section(LinearLayout parent,String s){TextView t=label(s,11,MUTED,true);t.setLetterSpacing(.08f);t.setPadding(dp(2),dp(9),0,dp(5));parent.addView(t);}
    private EditText field(String hint){EditText e=new EditText(this);e.setTextColor(TEXT);e.setHintTextColor(0xFF778397);e.setHint(hint);e.setTextSize(14);e.setPadding(dp(13),dp(10),dp(13),dp(10));e.setBackground(round(0xFF1A2230,13));LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-1,-2);p.bottomMargin=dp(8);e.setLayoutParams(p);return e;}
    private Button button(String s){Button b=new Button(this);b.setText(s);b.setTextColor(Color.WHITE);b.setTextSize(14);b.setTypeface(null,Typeface.BOLD);b.setAllCaps(false);b.setMinHeight(dp(48));b.setBackground(round(BLUE,25));return b;}
    private TextView label(String s,int size,int color,boolean bold){TextView t=new TextView(this);t.setText(s);t.setTextSize(size);t.setTextColor(color);if(bold)t.setTypeface(null,Typeface.BOLD);return t;}
    private android.graphics.drawable.GradientDrawable round(int color,int radius){android.graphics.drawable.GradientDrawable d=new android.graphics.drawable.GradientDrawable();d.setColor(color);d.setCornerRadius(dp(radius));return d;}
    private int dp(float n){return (int)(n*getResources().getDisplayMetrics().density+0.5f);}
    private String enc(String s){try{return URLEncoder.encode(s,"UTF-8");}catch(Exception e){return s;}}
    private void toast(String s){KentDialog.showToast(this,s);}
    @Override protected void onDestroy(){super.onDestroy();io.shutdown();}
}
