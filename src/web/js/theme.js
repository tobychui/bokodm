/*
 * theme.js
 *
 * Handles dark/light mode persistence and toggling.
 * restoreDarkMode() is called immediately (before body renders) to avoid a
 * flash of the wrong theme, so it uses native DOM instead of jQuery to avoid
 * any dependency on jQuery's document-ready state.
 */

function restoreDarkMode() {
    try {
        var html = document.documentElement;
        if (localStorage.getItem("darkMode") === "enabled") {
            html.classList.add("is-dark");
            html.classList.remove("is-white");
        } else {
            html.classList.remove("is-dark");
            html.classList.add("is-white");
        }
    } catch (e) {
        // localStorage may be unavailable (Safari private mode, etc.)
        // Fall back silently — the page still renders in its default theme.
    }
}
restoreDarkMode();

function updateElementToTheme(isDarkTheme) {
    isDarkTheme = !!isDarkTheme;
    if (!isDarkTheme) {
        $("#sysicon").attr("src", "./img/logo.svg");
        $("#darkModeToggle").html('<span class="ts-icon is-sun-icon"></span>');

        if (typeof changeScaleTextColor === "function") {
            changeScaleTextColor("black");
        }
    } else {
        $("#sysicon").attr("src", "./img/logo_white.svg");
        $("#darkModeToggle").html('<span class="ts-icon is-moon-icon"></span>');

        if (typeof changeScaleTextColor === "function") {
            changeScaleTextColor("white");
        }
    }
}

$(document).ready(function () {
    // Re-apply theme after full DOM parse so icons are in sync with the
    // class that restoreDarkMode() already set on <html>.
    var isDark = false;
    try {
        isDark = localStorage.getItem("darkMode") === "enabled";
    } catch (e) {}
    updateElementToTheme(isDark);

    $("#darkModeToggle").on("click", function (e) {
        e.preventDefault();
        var html = document.documentElement;
        var nowDark = !html.classList.contains("is-dark");
        html.classList.toggle("is-dark", nowDark);
        html.classList.toggle("is-white", !nowDark);
        try {
            localStorage.setItem("darkMode", nowDark ? "enabled" : "disabled");
        } catch (e) {}
        updateElementToTheme(nowDark);
    });
});
