package com.kentgram;

import android.app.Activity;
import android.graphics.Color;
import android.graphics.Typeface;
import android.os.Bundle;
import android.view.Gravity;
import android.view.View;
import android.widget.*;
import android.content.SharedPreferences;
import android.content.Intent;
import org.json.*;
import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public final class MainActivity extends Activity {
    private static final String API="https://api.kentgram.tendercheck.pro/api/v1";
    private static final int BG=0xFF0D1118,PANEL=0xFF151C27,TEXT=0xFFF4F7FB,MUTED=0xFF9AA9BC;
    private final ExecutorService io=Executors.newSingleThreadExecutor();
    private SharedPreferences prefs;
    private String token="";
    private LinearLayout root,content,top;
    private EditText composer;
    private long currentChat=0;

    @Override public void onCreate(Bundle state){super.onCreate(state);getWindow().setStatusBarColor(Color.BLACK);getWindow().setNavigationBarColor(BG);prefs=getSharedPreferences("kentgram",MODE_PRIVATE);token=prefs.getString("access_token","");if(token.isEmpty())showAuth();else showChats();}

    private void base(String title){
        root=new LinearLayout(this);root.setOrientation(1);root.setBackgroundColor(BG);
        top=new LinearLayout(this);top.setGravity(Gravity.CENTER_VERTICAL);top.setPadding(dp(18),dp(12),dp(18),dp(12));top.setBackgroundColor(PANEL);
        TextView h=label(title,21,TEXT,true);top.addView(h,new LinearLayout.LayoutParams(0,-2,1));root.addView(top,new LinearLayout.LayoutParams(-1,-2));
        ScrollView scroll=new ScrollView(this);content=new LinearLayout(this);content.setOrientation(1);content.setPadding(dp(14),dp(12),dp(14),dp(12));scroll.addView(content);root.addView(scroll,new LinearLayout.LayoutParams(-1,0,1));setContentView(root);
    }
    private void showAuth(){
        base("kentGram");TextView info=label("Общение начинается здесь",16,MUTED,true);info.setPadding(0,dp(18),0,dp(18));content.addView(info);
        EditText email=field("Email"),password=field("Пароль"),username=field("Имя пользователя — для регистрации");password.setInputType(129);content.addView(email);content.addView(password);content.addView(username);
        Button login=button("Войти"),register=button("Создать аккаунт");content.addView(login);content.addView(register);
        login.setOnClickListener(v->auth(email.getText().toString(),password.getText().toString(),null));
        register.setOnClickListener(v->auth(email.getText().toString(),password.getText().toString(),username.getText().toString()));
    }
    private void auth(String email,String password,String username){
        JSONObject b=new JSONObject();try{b.put("email",email.trim());b.put("password",password);b.put("device_name","kentGram Android");if(username!=null)b.put("username",username.trim());}catch(Exception ignored){}
        request(username==null?"/auth/login":"/auth/register",b,false,r->{token=r.optString("access_token","");if(token.isEmpty()){toast("Сервер не вернул токен");return;}prefs.edit().putString("access_token",token).putString("refresh_token",r.optString("refresh_token","")).apply();showChats();});
    }
    private void showChats(){
        currentChat=0;base("kentGram");Button profile=button("Мой профиль");content.addView(profile);profile.setOnClickListener(v->startActivity(new Intent(this,ProfileActivity.class)));Button find=button("＋  Новая переписка");content.addView(find);find.setOnClickListener(v->showUserSearch());
        TextView caption=label("Сообщения",14,MUTED,true);caption.setPadding(0,dp(16),0,dp(8));content.addView(caption);loadChats();
        Button logout=button("Выйти");content.addView(logout);logout.setOnClickListener(v->{request("/auth/logout",new JSONObject(),true,r->{});prefs.edit().clear().apply();token="";showAuth();});
    }
    private void loadChats(){
        requestGet("/chats",r->{if(content.getChildCount()>2)content.removeViews(2,content.getChildCount()-2);JSONArray a=r.optJSONArray("chats");
            if(a==null||a.length()==0){content.addView(label("Пока нет переписок. Нажми «Новая переписка», чтобы найти пользователя.",15,MUTED,false));return;}
            for(int i=0;i<a.length();i++){JSONObject c=a.optJSONObject(i);if(c==null)continue;long id=c.optLong("id");String name=c.optString("peer_display_name");if(name.isEmpty())name="@"+c.optString("peer_username");String last=c.optString("last_message","Начните общение");
                final long chatId=id;final String peerName=name;LinearLayout card=new LinearLayout(this);card.setOrientation(1);card.setPadding(dp(14),dp(12),dp(14),dp(12));card.setBackground(round(PANEL,14));TextView title=label(peerName,16,TEXT,true);TextView preview=label(last.isEmpty()?"Нет сообщений":last,14,MUTED,false);card.addView(title);card.addView(preview);LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(-1,-2);lp.bottomMargin=dp(8);content.addView(card,lp);card.setOnClickListener(v->openChat(chatId,peerName));}
        });
    }
    private void showUserSearch(){
        base("Новая переписка");EditText q=field("Имя пользователя");content.addView(q);Button search=button("Найти");content.addView(search);
        search.setOnClickListener(v->{String value=q.getText().toString().trim();if(value.length()<2){toast("Введите минимум 2 символа");return;}requestGet("/users/search?q="+enc(value),r->{if(content.getChildCount()>2)content.removeViews(2,content.getChildCount()-2);JSONArray a=r.optJSONArray("users");if(a==null||a.length()==0){content.addView(label("Пользователи не найдены",14,MUTED,false));return;}for(int i=0;i<a.length();i++){JSONObject u=a.optJSONObject(i);if(u==null)continue;long uid=u.optLong("id");String name=u.optString("display_name");String handle="@"+u.optString("username");LinearLayout row=new LinearLayout(this);row.setOrientation(1);row.setPadding(dp(10),dp(8),dp(10),dp(8));row.setBackground(round(PANEL,12));TextView caption=label((name.isEmpty()?handle:name+"  "+handle),15,TEXT,true);row.addView(caption);Button profile=button("Открыть профиль");row.addView(profile);profile.setOnClickListener(x->{Intent intent=new Intent(this,ProfileActivity.class);intent.putExtra("username",u.optString("username"));startActivity(intent);});Button message=button("Написать");row.addView(message);message.setOnClickListener(x->{JSONObject body=new JSONObject();try{body.put("user_id",uid);}catch(Exception ignored){}request("/chats/direct",body,true,created->openChat(created.optLong("chat_id"),name.isEmpty()?handle:name));});content.addView(row);}});});
    }
    private void openChat(long id,String peer){
        currentChat=id;base(peer);
        // Replace the generic list container with the dedicated message history.
        root.removeViewAt(1);
        Button back=button("‹ Чаты");back.setTextSize(13);LinearLayout.LayoutParams bp=new LinearLayout.LayoutParams(-2,-2);top.addView(back,bp);back.setOnClickListener(v->showChats());
        ScrollView history=new ScrollView(this);LinearLayout messages=new LinearLayout(this);messages.setOrientation(1);messages.setPadding(dp(10),dp(8),dp(10),dp(8));history.addView(messages);root.addView(history,new LinearLayout.LayoutParams(-1,0,1));
        LinearLayout bar=new LinearLayout(this);bar.setGravity(Gravity.CENTER_VERTICAL);bar.setPadding(dp(8),dp(7),dp(8),dp(7));bar.setBackgroundColor(PANEL);composer=field("Сообщение");bar.addView(composer,new LinearLayout.LayoutParams(0,-2,1));Button send=button("➤");send.setTextSize(16);LinearLayout.LayoutParams sp=new LinearLayout.LayoutParams(dp(58),dp(48));bar.addView(send,sp);root.addView(bar);
        send.setOnClickListener(v->sendMessage(id,messages,history));composer.setOnEditorActionListener((v,a,e)->{sendMessage(id,messages,history);return true;});refreshMessages(id,messages,history);
    }
    private void refreshMessages(long id,LinearLayout messages,ScrollView history){
        requestGet("/chats/"+id+"/messages",r->{if(currentChat!=id)return;messages.removeAllViews();JSONArray a=r.optJSONArray("messages");if(a!=null)for(int i=0;i<a.length();i++){JSONObject m=a.optJSONObject(i);if(m==null)continue;boolean mine=m.optBoolean("is_mine");LinearLayout row=new LinearLayout(this);row.setGravity(mine?Gravity.RIGHT:Gravity.LEFT);TextView bubble=label(m.optString("body"),15,TEXT,false);bubble.setPadding(dp(13),dp(9),dp(13),dp(9));bubble.setBackground(round(mine?0xFF17466A:0xFF202A38,16));LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-2,-2);p.setMargins(dp(2),dp(4),dp(2),dp(4));row.addView(bubble,p);messages.addView(row);}history.post(()->history.fullScroll(View.FOCUS_DOWN));});
    }
    private void sendMessage(long id,LinearLayout messages,ScrollView history){String text=composer.getText().toString().trim();if(text.isEmpty())return;composer.setText("");JSONObject b=new JSONObject();try{b.put("text",text);}catch(Exception ignored){}request("/chats/"+id+"/messages",b,true,r->refreshMessages(id,messages,history));}
    private interface Done{void accept(JSONObject value);}
    private void requestGet(String path,Done done){request(path,null,true,done);}
    private void request(String path,JSONObject body,boolean auth,Done done){
        io.execute(()->{JSONObject out=new JSONObject();int status=0;try{HttpURLConnection c=(HttpURLConnection)new URL(API+path).openConnection();c.setRequestMethod(body==null?"GET":"POST");c.setConnectTimeout(12000);c.setReadTimeout(12000);c.setRequestProperty("Accept","application/json");if(auth&&!token.isEmpty())c.setRequestProperty("Authorization","Bearer "+token);if(body!=null){c.setDoOutput(true);c.setRequestProperty("Content-Type","application/json");try(OutputStream s=c.getOutputStream()){s.write(body.toString().getBytes(StandardCharsets.UTF_8));}}status=c.getResponseCode();InputStream in=status>=200&&status<300?c.getInputStream():c.getErrorStream();if(in!=null){ByteArrayOutputStream b=new ByteArrayOutputStream();byte[] buf=new byte[4096];int n;while((n=in.read(buf))>0)b.write(buf,0,n);out=new JSONObject(b.toString("UTF-8"));}c.disconnect();}catch(Exception e){try{out.put("error","network_error");}catch(Exception ignored){}}final JSONObject result=out;final int code=status;runOnUiThread(()->{if(code>=200&&code<300)done.accept(result);else if(code==401&&auth){prefs.edit().clear().apply();token="";showAuth();toast("Войдите снова");}else toast(result.optString("error","Ошибка соединения")+" ("+code+")");});});
    }
    private String enc(String s){try{return URLEncoder.encode(s,"UTF-8");}catch(Exception e){return s;}}
    private EditText field(String hint){EditText e=new EditText(this);e.setTextColor(TEXT);e.setHintTextColor(MUTED);e.setHint(hint);e.setPadding(dp(13),dp(8),dp(13),dp(8));e.setBackground(round(PANEL,12));LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-1,-2);p.bottomMargin=dp(9);e.setLayoutParams(p);return e;}
    private Button button(String s){Button b=new Button(this);b.setText(s);b.setTextColor(TEXT);b.setAllCaps(false);b.setBackground(round(0xFF17649A,12));LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-1,-2);p.bottomMargin=dp(8);b.setLayoutParams(p);return b;}
    private TextView label(String s,int size,int color,boolean bold){TextView t=new TextView(this);t.setText(s);t.setTextSize(size);t.setTextColor(color);if(bold)t.setTypeface(null,Typeface.BOLD);return t;}
    private android.graphics.drawable.GradientDrawable round(int color,int radius){android.graphics.drawable.GradientDrawable d=new android.graphics.drawable.GradientDrawable();d.setColor(color);d.setCornerRadius(dp(radius));return d;}
    private int dp(float n){return (int)(n*getResources().getDisplayMetrics().density+0.5f);}
    private void toast(String s){Toast.makeText(this,s,Toast.LENGTH_LONG).show();}
    @Override protected void onDestroy(){super.onDestroy();io.shutdown();}
}