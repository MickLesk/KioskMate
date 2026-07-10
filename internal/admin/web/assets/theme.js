(() => {
        const stored = localStorage.getItem("kioskmate.theme");
        document.documentElement.dataset.theme = stored === "light" ? "light" : "dark";
      })();
