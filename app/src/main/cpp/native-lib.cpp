#include <jni.h>
#include <string>

extern "C"
JNIEXPORT jstring JNICALL
Java_com_kentgram_MainActivity_nativeCoreStatus(JNIEnv* env, jobject /* thiz */) {
    const std::string status = "C++ / JNI core подключён";
    return env->NewStringUTF(status.c_str());
}
