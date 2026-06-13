# Zenith Eclipse ERP Ultimate - Android APK Source

The real APK must be built with Android Studio because Android SDK/Gradle build tools are not available in this chat environment.

## Build steps

1. Open the `android` folder in Android Studio.
2. Open `app/src/main/res/values/strings.xml`.
3. Change this value:

```xml
<string name="erp_url">http://YOUR-SERVER-IP:8080</string>
```

For a VPS/domain, use for example:

```xml
<string name="erp_url">https://erp.yourdomain.com</string>
```

For office Wi-Fi testing, use for example:

```xml
<string name="erp_url">http://192.168.1.20:8080</string>
```

4. In Android Studio choose **Build > Generate App Bundles or APKs > Generate APK**.
5. Install the APK on employee phones.

The APK is a WebView wrapper that opens your server app, so all employees share the same database and can register accounts for admin approval.
