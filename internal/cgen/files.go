package cgen

const fileWriteRuntime = `
#include <errno.h>
#ifdef _WIN32
#include <windows.h>
#endif

static bool gors_replace_file(const char* from, const char* to) {
#ifdef _WIN32
    return MoveFileExA(from, to, MOVEFILE_REPLACE_EXISTING) != 0;
#else
    return rename(from, to) == 0;
#endif
}

[[maybe_unused]] static Result_bool_str gors_write_text(const char* path, const char* text) {
    const char* error = "could not write file";
    size_t path_len = strlen(path);
    if (path_len == 0 || path_len > SIZE_MAX - 32)
        return (Result_bool_str){ .err = gors_str_new(error) };
    size_t directory = 0;
    for (size_t i = 0; i < path_len; ++i) {
        if (path[i] == '/') directory = i + 1;
#ifdef _WIN32
        if (path[i] == '\\') directory = i + 1;
#endif
    }
#ifdef _WIN32
    if (directory == 0 && path_len >= 2 && path[1] == ':') directory = 2;
#endif
    char* temp = gors_malloc(directory + 32);
    memcpy(temp, path, directory);
    FILE* file = NULL;
    for (int i = 0; i < 128; ++i) {
        snprintf(temp + directory, 32, ".gors-tmp-%d", i);
#ifdef _WIN32
        if (lstrcmpiA(temp, path) == 0) continue;
#else
        if (strcmp(temp, path) == 0) continue;
#endif
        file = fopen(temp, "wbx");
        if (file || errno != EEXIST) break;
    }
    bool ok = false;
    if (file) {
        size_t size = strlen(text);
        ok = fwrite(text, 1, size, file) == size;
        if (fclose(file) != 0) ok = false;
        if (ok) ok = gors_replace_file(temp, path);
        if (!ok) remove(temp);
    }
    gors_free(temp);
    if (!ok) return (Result_bool_str){ .err = gors_str_new(error) };
    return (Result_bool_str){ .ok = true, .val = true };
}
`
