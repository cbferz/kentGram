package com.kentgram;

import android.app.Activity;
import android.app.Dialog;
import android.content.Context;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.graphics.drawable.ColorDrawable;
import android.os.Handler;
import android.os.Looper;
import android.view.Gravity;
import android.view.Window;
import android.view.WindowManager;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.TextView;

public final class KentDialog {
    private static final int PANEL=0xFF151C27;
    private static final int TEXT=0xFFF4F7FB;
    private static final int MUTED=0xFF9AA9BC;
    private static final int BLUE=0xFF2878C7;

    private KentDialog() {}

    public static void showMessage(Activity activity, String title, String message,
                                   String action, Runnable onAction) {
        if (activity == null || activity.isFinishing()) return;
        Dialog dialog = new Dialog(activity);
        LinearLayout card = new LinearLayout(activity);
        card.setOrientation(LinearLayout.VERTICAL);
        card.setPadding(dp(activity, 22), dp(activity, 22), dp(activity, 22), dp(activity, 18));
        card.setBackground(shape(PANEL, dp(activity, 26), 0xFF2A3749));

        TextView brand = text(activity, "KENTGRAM  /  УВЕДОМЛЕНИЕ", 10, 0xFF76A7FF, true);
        card.addView(brand);

        TextView heading = text(activity, title, 20, TEXT, true);
        LinearLayout.LayoutParams hp = new LinearLayout.LayoutParams(-1, -2);
        hp.topMargin = dp(activity, 12);
        card.addView(heading, hp);

        TextView body = text(activity, message, 14, MUTED, false);
        body.setLineSpacing(dp(activity, 3), 1.0f);
        LinearLayout.LayoutParams bp = new LinearLayout.LayoutParams(-1, -2);
        bp.topMargin = dp(activity, 9);
        card.addView(body, bp);

        Button ok = new Button(activity);
        ok.setText(action == null ? "Понятно" : action);
        ok.setTextColor(TEXT);
        ok.setTextSize(14);
        ok.setTypeface(null, Typeface.BOLD);
        ok.setAllCaps(false);
        ok.setMinHeight(dp(activity, 50));
        ok.setBackground(shape(BLUE, dp(activity, 28), BLUE));
        LinearLayout.LayoutParams op = new LinearLayout.LayoutParams(-1, -2);
        op.topMargin = dp(activity, 20);
        card.addView(ok, op);
        ok.setOnClickListener(v -> {
            dialog.dismiss();
            if (onAction != null) onAction.run();
        });

        dialog.setContentView(card);
        dialog.setCancelable(true);
        Window window = dialog.getWindow();
        if (window != null) {
            window.setBackgroundDrawable(new ColorDrawable(Color.TRANSPARENT));
            window.addFlags(WindowManager.LayoutParams.FLAG_DIM_BEHIND);
            WindowManager.LayoutParams attrs = window.getAttributes();
            attrs.width = activity.getResources().getDisplayMetrics().widthPixels - dp(activity, 44);
            attrs.height = WindowManager.LayoutParams.WRAP_CONTENT;
            attrs.gravity = Gravity.CENTER;
            attrs.dimAmount = 0.62f;
            window.setAttributes(attrs);
        }
        dialog.show();
        if (dialog.getWindow() != null) {
            dialog.getWindow().setLayout(
                    activity.getResources().getDisplayMetrics().widthPixels - dp(activity, 44),
                    WindowManager.LayoutParams.WRAP_CONTENT);
        }
    }

    public static void showToast(Activity activity, String message) {
        if (activity == null || activity.isFinishing()) return;
        Dialog dialog = new Dialog(activity);
        TextView pill = text(activity, message, 13, TEXT, true);
        pill.setPadding(dp(activity, 20), dp(activity, 14), dp(activity, 20), dp(activity, 14));
        pill.setBackground(shape(0xFF202A38, dp(activity, 30), 0xFF35465D));
        dialog.setContentView(pill);
        dialog.setCancelable(false);
        Window window = dialog.getWindow();
        if (window != null) {
            window.setBackgroundDrawable(new ColorDrawable(Color.TRANSPARENT));
            window.addFlags(WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE);
            window.setDimAmount(0f);
            WindowManager.LayoutParams attrs = window.getAttributes();
            attrs.width = WindowManager.LayoutParams.WRAP_CONTENT;
            attrs.height = WindowManager.LayoutParams.WRAP_CONTENT;
            attrs.gravity = Gravity.BOTTOM | Gravity.CENTER_HORIZONTAL;
            attrs.y = dp(activity, 38);
            window.setAttributes(attrs);
        }
        dialog.show();
        if (dialog.getWindow() != null) {
            dialog.getWindow().setBackgroundDrawable(new ColorDrawable(Color.TRANSPARENT));
        }
        new Handler(Looper.getMainLooper()).postDelayed(() -> {
            if (dialog.isShowing()) dialog.dismiss();
        }, 2300);
    }

    private static TextView text(Context context, String value, int size, int color, boolean bold) {
        TextView view = new TextView(context);
        view.setText(value);
        view.setTextSize(size);
        view.setTextColor(color);
        if (bold) view.setTypeface(null, Typeface.BOLD);
        return view;
    }

    private static GradientDrawable shape(int color, float radius, int stroke) {
        GradientDrawable drawable = new GradientDrawable();
        drawable.setColor(color);
        drawable.setCornerRadius(radius);
        if (stroke != color) drawable.setStroke(1, stroke);
        return drawable;
    }

    private static int dp(Context context, float value) {
        return (int) (value * context.getResources().getDisplayMetrics().density + 0.5f);
    }
}
