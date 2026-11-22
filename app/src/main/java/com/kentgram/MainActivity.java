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
    private LinearLayout root,content,top,navBar;
    private EditText composer;
    private long currentChat=0;

    @Override public void onCreate(Bundle state){super.onCreate(state);getWindow().setStatusBarColor(Color.BLACK);getWindow().setNavigationBarColor(BG);prefs=getSharedPreferences("kentgram",MODE_PRIVATE);token=prefs.getString("access_token","");if(token.isEmpty())showAuth();else showChats();}

    private void base(String title){
        root=new LinearLayout(this);root.setOrientation(1);root.setBackgroundColor(BG);
        top=new LinearLayout(this);top.setGravity(Gravity.CENTER_VERTICAL);top.setPadding(dp(18),dp(12),dp(18),dp(12));top.setBackground(round(PANEL,22));
        TextView h=label(title,21,TEXT,true);top.addView(h,new LinearLayout.LayoutParams(0,-2,1));LinearLayout.LayoutParams headerParams=new LinearLayout.LayoutParams(-1,-2);headerParams.setMargins(dp(10),dp(8),dp(10),dp(4));root.addView(top,headerParams);
        ScrollView scroll=new ScrollView(this);content=new LinearLayout(this);content.setOrientation(1);content.setPadding(dp(16),dp(16),dp(16),dp(16));scroll.addView(content);root.addView(scroll,new LinearLayout.LayoutParams(-1,0,1));
        buildNavigation(title);
        setContentView(root);
    }

    private void buildNavigation(String title){
        navBar=new LinearLayout(this);
        navBar.setGravity(Gravity.CENTER);
        navBar.setOrientation(LinearLayout.HORIZONTAL);
        navBar.setPadding(dp(5),dp(5),dp(5),dp(5));
        navBar.setBackground(round(0xFF202124,32));

        String[] labels={"Чаты","Контакты","Лента","Настройки","Профиль"};
        String[] icons={"▤","♧","▧","⚙","●"};
        String[] screens={"chats","contacts","feed","settings","profile"};
        String active=title.equals("kentGram")?"chats":title.equals("Контакты")?"contacts":
                title.equals("Лента")?"feed":title.equals("Настройки")?"settings":"";

        for(int i=0;i<labels.length;i++){
            final String screen=screens[i];
            boolean selected=screen.equals(active);
            LinearLayout item=new LinearLayout(this);
            item.setGravity(Gravity.CENTER);
            item.setOrientation(1);
            item.setPadding(dp(1),dp(5),dp(1),dp(5));
            item.setBackground(round(selected?0xFF293246:Color.TRANSPARENT,26));

            TextView icon=label(icons[i],22,selected?0xFF5685FF:0xFFE4E8F0,true);
            icon.setGravity(Gravity.CENTER);
            TextView text=label(labels[i],10,selected?0xFF5685FF:0xFFE4E8F0,true);
            text.setGravity(Gravity.CENTER);
            text.setMaxLines(1);
            text.setIncludeFontPadding(false);
            item.addView(icon);
            item.addView(text);

            LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(0,dp(58),1);
            lp.setMargins(dp(1),0,dp(1),0);
            navBar.addView(item,lp);
            item.setContentDescription(labels[i]);
            item.setOnClickListener(v->navigate(screen));
        }
        LinearLayout.LayoutParams navParams=new LinearLayout.LayoutParams(-1,-2);
        navParams.setMargins(dp(10),dp(4),dp(10),dp(7));
        root.addView(navBar,navParams);
    }
    private void navigate(String screen){
        switch(screen){
            case "chats":showChats();break;
            case "contacts":showContacts();break;
            case "feed":showFeed();break;
            case "settings":showSettings();break;
            case "profile":startActivity(new Intent(this,ProfileActivity.class));break;
        }
    }
    private void showContacts(){
        base("Контакты");
        TextView heading=label("Мои контакты",17,TEXT,true);content.addView(heading);
        LinearLayout contactsList=new LinearLayout(this);contactsList.setOrientation(1);content.addView(contactsList);
        requestGet("/contacts",r->{
            contactsList.removeAllViews();JSONArray contacts=r.optJSONArray("contacts");
            if(contacts==null||contacts.length()==0){contactsList.addView(label("Пока нет контактов. Найди пользователя ниже и открой его профиль, чтобы подписаться.",13,MUTED,false));}
            else for(int i=0;i<contacts.length();i++){
                JSONObject u=contacts.optJSONObject(i);if(u==null)continue;
                String username=u.optString("username"),name=u.optString("display_name");
                LinearLayout card=new LinearLayout(this);card.setOrientation(1);card.setPadding(dp(13),dp(10),dp(13),dp(10));card.setBackground(round(PANEL,13));
                card.addView(label(name.isEmpty()?"@"+username:name,15,TEXT,true));
                card.addView(label("@"+username,12,MUTED,false));
                Button open=button("Профиль");card.addView(open);
                open.setOnClickListener(v->{Intent in=new Intent(this,ProfileActivity.class);in.putExtra("username",username);startActivity(in);});
                LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(-1,-2);lp.bottomMargin=dp(7);contactsList.addView(card,lp);
            }
        });
        TextView discover=label("Найти людей",17,TEXT,true);discover.setPadding(0,dp(18),0,dp(8));content.addView(discover);
        EditText query=field("Имя или username");content.addView(query);
        Button search=button("Найти пользователей");content.addView(search);
        LinearLayout results=new LinearLayout(this);results.setOrientation(1);content.addView(results);
        search.setOnClickListener(v->{
            String q=query.getText().toString().trim();if(q.length()<2){toast("Введите минимум 2 символа");return;}
            search.setEnabled(false);
            requestGet("/users/search?q="+enc(q),r->{
                search.setEnabled(true);results.removeAllViews();JSONArray users=r.optJSONArray("users");
                if(users==null||users.length()==0){results.addView(label("Ничего не найдено",14,MUTED,false));return;}
                for(int i=0;i<users.length();i++){
                    JSONObject u=users.optJSONObject(i);if(u==null)continue;
                    String username=u.optString("username"),name=u.optString("display_name");
                    LinearLayout card=new LinearLayout(this);card.setOrientation(1);card.setPadding(dp(13),dp(11),dp(13),dp(11));card.setBackground(round(PANEL,14));
                    card.addView(label(name.isEmpty()?"@"+username:name,16,TEXT,true));
                    card.addView(label("@"+username,13,MUTED,false));
                    Button open=button("Открыть профиль");card.addView(open);
                    open.setOnClickListener(x->{Intent in=new Intent(this,ProfileActivity.class);in.putExtra("username",username);startActivity(in);});
                    LinearLayout.LayoutParams cp=new LinearLayout.LayoutParams(-1,-2);cp.bottomMargin=dp(8);results.addView(card,cp);
                }
            });
        });
    }
    private void showFeed(){
        base("Лента");
        content.addView(label("Публикации появятся следующим этапом",17,TEXT,true));
        content.addView(label("Сейчас готовим API публикаций, реакции и комментарии. Пока здесь нет демонстрационных или выдуманных постов.",14,MUTED,false));
    }
    private void showSettings(){
        base("Настройки");
        content.addView(label("Аккаунт",13,MUTED,true));
        Button profile=button("Мой профиль");content.addView(profile);profile.setOnClickListener(v->startActivity(new Intent(this,ProfileActivity.class)));
        Button logout=button("Выйти из аккаунта");content.addView(logout);
        logout.setOnClickListener(v->{request("/auth/logout",new JSONObject(),true,r->{});prefs.edit().clear().apply();token="";showAuth();});
    }

    private void showAuth(){
        base("kentGram");
        if(navBar!=null)root.removeView(navBar);
        content.setPadding(dp(20),dp(24),dp(20),dp(28));

        LinearLayout hero=new LinearLayout(this);
        hero.setOrientation(1);
        hero.setGravity(Gravity.CENTER);
        hero.setPadding(dp(20),dp(26),dp(20),dp(24));
        hero.setBackground(round(PANEL,26));
        TextView mark=label("k",38,0xFF76A7FF,true);
        mark.setGravity(Gravity.CENTER);
        mark.setBackground(round(0xFF203A61,28));
        LinearLayout.LayoutParams mp=new LinearLayout.LayoutParams(dp(76),dp(76));
        mp.bottomMargin=dp(16);
        hero.addView(mark,mp);
        TextView title=label("Твой круг общения",23,TEXT,true);
        title.setGravity(Gravity.CENTER);
        hero.addView(title);
        TextView subtitle=label("Общайся. Находи своих. Оставайся на связи.",13,MUTED,false);
        subtitle.setGravity(Gravity.CENTER);
        subtitle.setPadding(0,dp(8),0,dp(2));
        hero.addView(subtitle);
        content.addView(hero);

        LinearLayout form=new LinearLayout(this);
        form.setOrientation(1);
        form.setPadding(dp(18),dp(20),dp(18),dp(18));
        form.setBackground(round(0xFF111925,24));
        LinearLayout.LayoutParams fp=new LinearLayout.LayoutParams(-1,-2);
        fp.topMargin=dp(16);
        content.addView(form,fp);

        TextView formTitle=label("Вход в аккаунт",18,TEXT,true);
        formTitle.setPadding(0,0,0,dp(14));
        form.addView(formTitle);
        EditText email=field("Email");
        EditText password=field("Пароль");
        password.setInputType(129);
        EditText username=field("Имя пользователя");
        username.setVisibility(View.GONE);
        form.addView(email);
        form.addView(password);
        form.addView(username);

        Button primary=button("Войти");
        Button secondary=button("Создать аккаунт");
        primary.setTextSize(16);
        primary.setMinHeight(dp(60));
        primary.setBackground(round(0xFF2878C7,40));
        secondary.setTextSize(14);
        secondary.setMinHeight(dp(48));
        secondary.setBackground(round(0xFF202A38,40));
        form.addView(primary);
        LinearLayout.LayoutParams secondaryParams=(LinearLayout.LayoutParams)secondary.getLayoutParams();
        if(secondaryParams==null)secondaryParams=new LinearLayout.LayoutParams(-1,-2);
        secondaryParams.topMargin=dp(4);
        secondary.setLayoutParams(secondaryParams);
        form.addView(secondary);

        final boolean[] registering={false};
        primary.setOnClickListener(v->{
            String mail=email.getText().toString().trim();
            String pass=password.getText().toString();
            if(mail.isEmpty()||pass.isEmpty()){
                KentDialog.showMessage(this,"Заполни поля","Укажи email и пароль, чтобы продолжить.","Понятно",null);
                return;
            }
            if(registering[0]&&username.getText().toString().trim().isEmpty()){
                KentDialog.showMessage(this,"Нужно имя пользователя","Придумай username для нового аккаунта.","Понятно",null);
                return;
            }
            auth(mail,pass,registering[0]?username.getText().toString():null);
        });
        secondary.setOnClickListener(v->{
            registering[0]=!registering[0];
            username.setVisibility(registering[0]?View.VISIBLE:View.GONE);
            formTitle.setText(registering[0]?"Создание аккаунта":"Вход в аккаунт");
            primary.setText(registering[0]?"Зарегистрироваться":"Войти");
            secondary.setText(registering[0]?"Уже есть аккаунт? Войти":"Создать аккаунт");
        });
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
        root.removeViewAt(1);if(navBar!=null)root.removeView(navBar);
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
        io.execute(()->{JSONObject out=new JSONObject();int status=0;try{HttpURLConnection c=(HttpURLConnection)new URL(API+path).openConnection();c.setRequestMethod(body==null?"GET":"POST");c.setConnectTimeout(12000);c.setReadTimeout(12000);c.setRequestProperty("Accept","application/json");if(auth&&!token.isEmpty())c.setRequestProperty("Authorization","Bearer "+token);if(body!=null){c.setDoOutput(true);c.setRequestProperty("Content-Type","application/json");try(OutputStream s=c.getOutputStream()){s.write(body.toString().getBytes(StandardCharsets.UTF_8));}}status=c.getResponseCode();InputStream in=status>=200&&status<300?c.getInputStream():c.getErrorStream();if(in!=null){ByteArrayOutputStream b=new ByteArrayOutputStream();byte[] buf=new byte[4096];int n;while((n=in.read(buf))>0)b.write(buf,0,n);String responseText=b.toString("UTF-8");try{out=new JSONObject(responseText);}catch(JSONException parseError){out=new JSONObject();out.put("error",status==404?"endpoint_not_found":status>=500?"server_error":"http_error");}}c.disconnect();}catch(Exception e){try{out.put("error","connection_failed");}catch(Exception ignored){}}final JSONObject result=out;final int code=status;runOnUiThread(()->{if(code>=200&&code<300)done.accept(result);else if(code==401&&auth){prefs.edit().clear().apply();token="";showAuth();toast("Войдите снова");}else KentDialog.showMessage(this,code==404?"Маршрут не найден":"Не удалось выполнить запрос",result.optString("error","Ошибка соединения")+" ("+code+")","Понятно",null);});});
    }
    private String enc(String s){try{return URLEncoder.encode(s,"UTF-8");}catch(Exception e){return s;}}
    private EditText field(String hint){EditText e=new EditText(this);e.setTextColor(TEXT);e.setHintTextColor(MUTED);e.setHint(hint);e.setPadding(dp(13),dp(8),dp(13),dp(8));e.setBackground(round(PANEL,12));LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-1,-2);p.bottomMargin=dp(9);e.setLayoutParams(p);return e;}
    private Button button(String s){Button b=new Button(this);b.setText(s);b.setTextColor(TEXT);b.setTextSize(14);b.setTypeface(null,Typeface.BOLD);b.setAllCaps(false);b.setMinHeight(dp(48));b.setPadding(dp(18),dp(8),dp(18),dp(8));b.setBackground(round(0xFF17649A,28));LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-1,-2);p.bottomMargin=dp(8);b.setLayoutParams(p);return b;}
    private TextView label(String s,int size,int color,boolean bold){TextView t=new TextView(this);t.setText(s);t.setTextSize(size);t.setTextColor(color);if(bold)t.setTypeface(null,Typeface.BOLD);return t;}
    private android.graphics.drawable.GradientDrawable round(int color,int radius){android.graphics.drawable.GradientDrawable d=new android.graphics.drawable.GradientDrawable();d.setColor(color);d.setCornerRadius(dp(radius));return d;}
    private int dp(float n){return (int)(n*getResources().getDisplayMetrics().density+0.5f);}
    private void toast(String s){KentDialog.showToast(this,s);}
    @Override protected void onDestroy(){super.onDestroy();io.shutdown();}
}