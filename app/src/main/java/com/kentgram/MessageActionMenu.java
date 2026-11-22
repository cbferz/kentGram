package com.kentgram;

import android.app.Activity;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Context;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.widget.LinearLayout;
import android.widget.PopupWindow;
import android.widget.TextView;
import android.widget.FrameLayout;

final class MessageActionMenu {
    private MessageActionMenu() {}

    static void show(Activity activity, View anchor, String message) {
        if (activity == null || anchor == null || message == null) return;

        View decor = activity.getWindow().getDecorView();
        View dim = new View(activity);
        dim.setBackgroundColor(0x99070B12);
        FrameLayout.LayoutParams dimParams = new FrameLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.MATCH_PARENT
        );
        FrameLayout frame = decor instanceof FrameLayout ? (FrameLayout) decor : null;
        if (frame != null) frame.addView(dim, dimParams);

        LinearLayout menu = new LinearLayout(activity);
        menu.setOrientation(LinearLayout.HORIZONTAL);
        menu.setGravity(Gravity.CENTER);
        menu.setPadding(dp(activity, 7), dp(activity, 7), dp(activity, 7), dp(activity, 7));
        GradientDrawable background = new GradientDrawable();
        background.setColor(0xFF202A38);
        background.setCornerRadius(dp(activity, 18));
        menu.setBackground(background);

        PopupWindow popup = new PopupWindow(menu, dp(activity, 190), dp(activity, 54), true);
        popup.setBackgroundDrawable(background);
        popup.setOutsideTouchable(true);
        popup.setElevation(dp(activity, 16));

        TextView copy = action(activity, "Копировать");
        menu.addView(copy, new LinearLayout.LayoutParams(0, -1, 1));
        copy.setOnClickListener(v -> {
            ClipboardManager clipboard = (ClipboardManager) activity.getSystemService(Context.CLIPBOARD_SERVICE);
            if (clipboard != null) {
                clipboard.setPrimaryClip(ClipData.newPlainText("kentGram message", message));
                android.widget.Toast.makeText(activity, "Сообщение скопировано", android.widget.Toast.LENGTH_SHORT).show();
            }
            popup.dismiss();
        });

        popup.setOnDismissListener(() -> {
            if (frame != null && dim.getParent() == frame) frame.removeView(dim);
        });

        int[] location = new int[2];
        anchor.getLocationOnScreen(location);
        int[] decorLocation = new int[2];
        decor.getLocationOnScreen(decorLocation);
        int x = Math.max(dp(activity, 12), location[0] + anchor.getWidth() / 2 - dp(activity, 95));
        int y = Math.max(dp(activity, 28), location[1] - dp(activity, 66));
        popup.showAtLocation(decor, Gravity.TOP | Gravity.LEFT, x, y);
        dim.setOnClickListener(v -> popup.dismiss());
        dim.bringToFront();
        // Keep the action surface above the dim layer.
        popup.getContentView().bringToFront();
    }

    private static TextView action(Activity activity, String title) {
        TextView view = new TextView(activity);
        view.setText(title);
        view.setTextColor(Color.WHITE);
        view.setTextSize(14);
        view.setTypeface(null, Typeface.BOLD);
        view.setGravity(Gravity.CENTER);
        view.setPadding(dp(activity, 10), 0, dp(activity, 10), 0);
        return view;
    }

    private static int dp(Activity activity, float value) {
        return (int) (value * activity.getResources().getDisplayMetrics().density + 0.5f);
    }
}
