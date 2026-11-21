package com.kentgram;

import android.app.Activity;
import android.graphics.Color;
import android.os.Bundle;
import android.view.Gravity;
import android.view.View;
import android.widget.LinearLayout;
import android.widget.TextView;

public final class MainActivity extends Activity {
    static {
        System.loadLibrary("kentgram_native");
    }

    private native String nativeCoreStatus();

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setGravity(Gravity.CENTER);
        root.setPadding(dp(28), dp(28), dp(28), dp(28));
        root.setBackgroundColor(Color.rgb(13, 17, 24));

        TextView logo = new TextView(this);
        logo.setText("kentGram");
        logo.setTextColor(Color.WHITE);
        logo.setTextSize(34);
        logo.setGravity(Gravity.CENTER);
        logo.setTypeface(null, android.graphics.Typeface.BOLD);

        TextView tagline = new TextView(this);
        tagline.setText("Общение. Контент. Сообщества.");
        tagline.setTextColor(Color.rgb(160, 174, 192));
        tagline.setTextSize(15);
        tagline.setGravity(Gravity.CENTER);
        tagline.setPadding(0, dp(12), 0, dp(28));

        TextView status = new TextView(this);
        status.setText("Android-клиент запущен\n" + nativeCoreStatus());
        status.setTextColor(Color.rgb(99, 179, 237));
        status.setTextSize(13);
        status.setGravity(Gravity.CENTER);

        root.addView(logo, matchWrap());
        root.addView(tagline, matchWrap());
        root.addView(status, matchWrap());

        setContentView(root);
    }

    private int dp(float value) {
        return (int) (value * getResources().getDisplayMetrics().density + 0.5f);
    }

    private LinearLayout.LayoutParams matchWrap() {
        return new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                LinearLayout.LayoutParams.WRAP_CONTENT
        );
    }
}
