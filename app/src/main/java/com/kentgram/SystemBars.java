package com.kentgram;

import android.app.Activity;
import android.graphics.Insets;
import android.graphics.drawable.ColorDrawable;
import android.os.Build;
import android.view.View;
import android.view.Window;
import android.view.WindowInsets;
import android.view.WindowInsetsController;

/**
 * Keeps every screen clear of the status bar, navigation bar, display cutout
 * and keyboard.
 *
 * With targetSdk 35, Android 15+ always draws apps edge to edge: the status
 * and navigation bar colours are ignored and adjustResize no longer shrinks
 * the window for the keyboard. Without this the header slides under the
 * clock and the composer hides behind the keyboard.
 */
final class SystemBars {
    private SystemBars(){}

    /** Call once in onCreate, before or after setContentView. */
    static void apply(Activity activity,int background){
        Window window=activity.getWindow();
        // Install the decor view first: before setContentView it does not exist
        // yet, and Window.getInsetsController() then throws a
        // NullPointerException on Android 11+.
        View decor=window.getDecorView();
        window.setBackgroundDrawable(new ColorDrawable(background));
        if(Build.VERSION.SDK_INT<Build.VERSION_CODES.R){
            // Before Android 11 the decor view already insets the content and
            // adjustResize handles the keyboard.
            window.setStatusBarColor(background);
            window.setNavigationBarColor(background);
            return;
        }
        window.setDecorFitsSystemWindows(false);
        if(Build.VERSION.SDK_INT<Build.VERSION_CODES.VANILLA_ICE_CREAM){
            window.setStatusBarColor(background);
            window.setNavigationBarColor(background);
        }
        window.setStatusBarContrastEnforced(false);
        window.setNavigationBarContrastEnforced(false);
        WindowInsetsController controller=decor.getWindowInsetsController();
        if(controller!=null){
            // Light icons on the dark background.
            controller.setSystemBarsAppearance(0,
                    WindowInsetsController.APPEARANCE_LIGHT_STATUS_BARS|
                    WindowInsetsController.APPEARANCE_LIGHT_NAVIGATION_BARS);
        }
        // android.R.id.content survives every setContentView call, so one
        // listener covers all screens the activity swaps in.
        View content=decor.findViewById(android.R.id.content);
        content.setOnApplyWindowInsetsListener((view,insets)->{
            Insets bars=insets.getInsets(WindowInsets.Type.systemBars()|
                    WindowInsets.Type.displayCutout()|WindowInsets.Type.ime());
            view.setPadding(bars.left,bars.top,bars.right,bars.bottom);
            return WindowInsets.CONSUMED;
        });
        content.requestApplyInsets();
    }
}
