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
import java.util.ArrayDeque;
import java.util.HashMap;
import java.util.HashSet;

public final class MainActivity extends Activity {
    private static final String API="https://api.kentgram.tendercheck.pro/api/v1";
    private static final int BG=0xFF0D1118,PANEL=0xFF151C27,TEXT=0xFFF4F7FB,MUTED=0xFF9AA9BC;
    private final ExecutorService io=Executors.newSingleThreadExecutor();
    private SharedPreferences prefs;
    private String token="";
    private LinearLayout root,content,top,navBar;
    private EditText composer;
    private long currentChat=0;
    private final HashMap<Long,ArrayDeque<String>> sendQueues=new HashMap<>();
    private final HashSet<Long> sendingChats=new HashSet<>();

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
        currentChat=0;
        base("kentGram");

        TextView searchTitle=label("Поиск людей",16,TEXT,true);
        content.addView(searchTitle);
        EditText query=field("Имя или @username");
        content.addView(query);
        Button search=button("Найти");
        content.addView(search);

        LinearLayout results=new LinearLayout(this);
        results.setOrientation(1);
        content.addView(results);

        TextView caption=label("Мои переписки",16,TEXT,true);
        caption.setPadding(0,dp(14),0,dp(9));
        content.addView(caption);
        LinearLayout chatsList=new LinearLayout(this);
        chatsList.setOrientation(1);
        content.addView(chatsList);
        loadChats(chatsList);

        search.setOnClickListener(v->searchUsers(query,search,results));
        query.setOnEditorActionListener((v,action,event)->{
            searchUsers(query,search,results);
            return true;
        });

        // Floating action button sits above the bottom navigation.
        LinearLayout fabBar=new LinearLayout(this);
        fabBar.setGravity(Gravity.RIGHT|Gravity.CENTER_VERTICAL);
        fabBar.setPadding(dp(16),dp(2),dp(18),dp(10));
        TextView fab=label("+",30,Color.WHITE,true);
        fab.setGravity(Gravity.CENTER);
        fab.setBackground(round(0xFF2878C7,50));
        fab.setElevation(dp(7));
        fab.setContentDescription("Новая переписка");
        fab.setOnClickListener(v->showUserSearch());
        fabBar.addView(fab,new LinearLayout.LayoutParams(dp(58),dp(58)));
        int navIndex=root.indexOfChild(navBar);
        root.addView(fabBar,navIndex<0?root.getChildCount():navIndex,
                new LinearLayout.LayoutParams(-1,-2));
    }

    private void loadChats(LinearLayout chatsList){
        requestGet("/chats",r->{
            chatsList.removeAllViews();
            JSONArray a=r.optJSONArray("chats");
            if(a==null||a.length()==0){
                chatsList.addView(label("Пока нет переписок. Начни новую через кнопку +.",14,MUTED,false));
                return;
            }
            for(int i=0;i<a.length();i++){
                JSONObject c=a.optJSONObject(i);
                if(c==null)continue;
                long id=c.optLong("id");
                String name=c.optString("peer_display_name");
                if(name.isEmpty())name="@"+c.optString("peer_username");
                String last=c.optString("last_message","Начните общение");
                final long chatId=id;
                final String peerName=name;
                LinearLayout card=new LinearLayout(this);
                card.setOrientation(1);
                card.setPadding(dp(14),dp(12),dp(14),dp(12));
                card.setBackground(round(PANEL,16));
                card.addView(label(peerName,16,TEXT,true));
                card.addView(label(last.isEmpty()?"Нет сообщений":last,14,MUTED,false));
                LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(-1,-2);
                lp.bottomMargin=dp(8);
                chatsList.addView(card,lp);
                card.setOnClickListener(v->openChat(chatId,peerName));
            }
        });
    }

    private void searchUsers(EditText query,Button search,LinearLayout results){
        String value=query.getText().toString().trim();
        if(value.startsWith("@"))value=value.substring(1).trim();
        if(value.length()<2){
            toast("Введите минимум 2 символа");
            return;
        }
        final String searchValue=value;
        search.setEnabled(false);
        requestGet("/users/search?q="+enc(searchValue),r->{
            search.setEnabled(true);
            results.removeAllViews();
            JSONArray users=r.optJSONArray("users");
            if(users==null||users.length()==0){
                results.addView(label("Пользователи не найдены",14,MUTED,false));
                return;
            }
            for(int i=0;i<users.length();i++){
                JSONObject u=users.optJSONObject(i);
                if(u==null)continue;
                long uid=u.optLong("id");
                String username=u.optString("username");
                String displayName=u.optString("display_name");
                String title=displayName.isEmpty()?"@"+username:displayName;
                LinearLayout card=new LinearLayout(this);
                card.setOrientation(1);
                card.setPadding(dp(13),dp(10),dp(13),dp(10));
                card.setBackground(round(PANEL,14));
                card.addView(label(title,15,TEXT,true));
                card.addView(label("@"+username,12,MUTED,false));
                Button message=button("Написать");
                card.addView(message);
                message.setOnClickListener(v->{
                    JSONObject body=new JSONObject();
                    try{body.put("user_id",uid);}catch(Exception ignored){}
                    request("/chats/direct",body,true,created->{
                        String peer=displayName.isEmpty()?"@"+username:displayName;
                        openChat(created.optLong("chat_id"),peer);
                    });
                });
                LinearLayout.LayoutParams lp=new LinearLayout.LayoutParams(-1,-2);
                lp.bottomMargin=dp(8);
                results.addView(card,lp);
            }
        });
    }

    private void showUserSearch(){
        base("Новая переписка");
        EditText q=field("Имя или @username");
        content.addView(q);
        Button search=button("Найти");
        content.addView(search);
        LinearLayout results=new LinearLayout(this);
        results.setOrientation(1);
        content.addView(results);
        search.setOnClickListener(v->searchUsers(q,search,results));
        q.setOnEditorActionListener((v,action,event)->{
            searchUsers(q,search,results);
            return true;
        });
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
        requestGet("/chats/"+id+"/messages",r->{
            if(currentChat!=id)return;
            messages.removeAllViews();
            JSONArray a=r.optJSONArray("messages");
            if(a!=null)for(int i=0;i<a.length();i++){
                JSONObject m=a.optJSONObject(i);
                if(m==null)continue;
                boolean mine=m.optBoolean("is_mine");
                long messageId=m.optLong("id",0);
                String body=m.optString("body","");
                LinearLayout row=new LinearLayout(this);
                row.setGravity(mine?Gravity.RIGHT:Gravity.LEFT);
                TextView bubble=label(body,15,TEXT,false);
                bubble.setPadding(dp(13),dp(9),dp(13),dp(9));
                bubble.setBackground(round(mine?0xFF17466A:0xFF202A38,16));
                LinearLayout.LayoutParams p=new LinearLayout.LayoutParams(-2,-2);
                p.setMargins(dp(2),dp(4),dp(2),dp(4));
                row.addView(bubble,p);
                bubble.setOnLongClickListener(v->{
                    MessageActionMenu.show(this,bubble,body,mine,
                            ()->confirmDeleteMessage(id,messageId,messages,history));
                    return true;
                });
                messages.addView(row);
            }
            history.post(()->history.fullScroll(View.FOCUS_DOWN));
        });
    }
    private void confirmDeleteMessage(long chatId,long messageId,LinearLayout messages,ScrollView history){
        if(messageId<=0)return;
        KentDialog.showMessage(this,"Удалить сообщение?",
                "Оно будет удалено у всех участников переписки.","Удалить",()->{
            requestDelete("/chats/"+chatId+"/messages/"+messageId,result->{
                if(currentChat==chatId)refreshMessages(chatId,messages,history);
                toast("Сообщение удалено");
            });
        });
    }
    private void sendMessage(long id,LinearLayout messages,ScrollView history){
        if(composer==null)return;
        String text=composer.getText().toString().trim();
        if(text.isEmpty())return;
        ArrayDeque<String> queue=sendQueues.computeIfAbsent(id,k->new ArrayDeque<>());
        // A failed head remains queued. Tapping send with that same restored draft retries it
        // instead of creating a duplicate message.
        if(queue.isEmpty()||!queue.peekFirst().equals(text)||sendingChats.contains(id))queue.addLast(text);
        composer.setText("");
        pumpSendQueue(id,messages,history);
    }
    private void pumpSendQueue(long id,LinearLayout messages,ScrollView history){
        if(sendingChats.contains(id))return;
        ArrayDeque<String> queue=sendQueues.get(id);
        if(queue==null||queue.isEmpty())return;
        String text=queue.peekFirst();
        sendingChats.add(id);
        JSONObject body=new JSONObject();
        try{body.put("text",text);}catch(Exception ignored){}
        request("/chats/"+id+"/messages",body,true,
            response->{
                ArrayDeque<String> pending=sendQueues.get(id);
                if(pending!=null&&!pending.isEmpty())pending.removeFirst();
                sendingChats.remove(id);
                if(pending!=null&&pending.isEmpty())sendQueues.remove(id);
                if(pending!=null&&!pending.isEmpty())pumpSendQueue(id,messages,history);
                else refreshMessages(id,messages,history);
            },
            (error,status)->{
                sendingChats.remove(id);
                if(currentChat==id&&composer!=null&&composer.getText().length()==0)composer.setText(text);
            });
    }
    private interface Done{void accept(JSONObject value);}
    private void requestGet(String path,Done done){request(path,null,true,done);}
    private interface Failed{void accept(JSONObject value,int status);}
    private void request(String path,JSONObject body,boolean auth,Done done){
        request(path,body,auth,done,null);
    }
    private void request(String path,JSONObject body,boolean auth,Done done,Failed failed){
        io.execute(()->{JSONObject out=new JSONObject();int status=0;try{HttpURLConnection c=(HttpURLConnection)new URL(API+path).openConnection();c.setRequestMethod(body==null?"GET":"POST");c.setConnectTimeout(12000);c.setReadTimeout(12000);c.setRequestProperty("Accept","application/json");if(auth&&!token.isEmpty())c.setRequestProperty("Authorization","Bearer "+token);if(body!=null){c.setDoOutput(true);c.setRequestProperty("Content-Type","application/json");try(OutputStream s=c.getOutputStream()){s.write(body.toString().getBytes(StandardCharsets.UTF_8));}}status=c.getResponseCode();InputStream in=status>=200&&status<300?c.getInputStream():c.getErrorStream();if(in!=null){ByteArrayOutputStream b=new ByteArrayOutputStream();byte[] buf=new byte[4096];int n;while((n=in.read(buf))>0)b.write(buf,0,n);String responseText=b.toString("UTF-8");try{out=new JSONObject(responseText);}catch(JSONException parseError){out=new JSONObject();out.put("error",status==404?"endpoint_not_found":status>=500?"server_error":"http_error");}}c.disconnect();}catch(Exception e){try{out.put("error","connection_failed");}catch(Exception ignored){}}final JSONObject result=out;final int code=status;runOnUiThread(()->{
            if(code>=200&&code<300){done.accept(result);return;}
            if(code==401&&auth){prefs.edit().clear().apply();token="";showAuth();toast("Войдите снова");}
            else KentDialog.showMessage(this,code==404?"Маршрут не найден":"Не удалось выполнить запрос",result.optString("error","Ошибка соединения")+" ("+code+")","Понятно",null);
            if(failed!=null)failed.accept(result,code);
        });});
    }
    private void requestDelete(String path,Done done){
        io.execute(()->{
            JSONObject out=new JSONObject();
            int status=0;
            try{
                HttpURLConnection c=(HttpURLConnection)new URL(API+path).openConnection();
                c.setRequestMethod("DELETE");
                c.setConnectTimeout(12000);
                c.setReadTimeout(12000);
                c.setRequestProperty("Accept","application/json");
                if(!token.isEmpty())c.setRequestProperty("Authorization","Bearer "+token);
                status=c.getResponseCode();
                InputStream in=status>=200&&status<300?c.getInputStream():c.getErrorStream();
                if(in!=null){
                    ByteArrayOutputStream b=new ByteArrayOutputStream();
                    byte[] buf=new byte[4096];
                    int n;
                    while((n=in.read(buf))>0)b.write(buf,0,n);
                    try{out=new JSONObject(b.toString("UTF-8"));}catch(JSONException ignored){}
                }
                c.disconnect();
            }catch(Exception e){
                try{out.put("error","connection_failed");}catch(Exception ignored){}
            }
            final JSONObject result=out;
            final int code=status;
            runOnUiThread(()->{
                if(code>=200&&code<300)done.accept(result);
                else if(code==401){
                    prefs.edit().clear().apply();
                    token="";
                    showAuth();
                }else{
                    KentDialog.showMessage(this,"Не удалось удалить сообщение",
                            result.optString("error","Ошибка соединения")+" ("+code+")","Понятно",null);
                }
            });
        });
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